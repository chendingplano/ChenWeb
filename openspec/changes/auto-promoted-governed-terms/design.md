## Context

ADR `2026081201` decided (after checking DR23's assumptions against the live
`miner` database) that `metric_definition` term creation must become
automatic rather than human-gated. Current state, verified directly against
code and data, not assumed:

- `kb.ontology_terms` has zero `metric_definition` rows; only QUDT-imported
  `quantity` content (units/quantity-kinds/dimensions) and 20 `core` rows
  exist.
- `kb.metrics.keyword_concept_id`/`.metric_definition_term_id` are `0` of
  `7,040` rows populated. `KEYWORD_RESOLVER_MODE` is unset in this
  deployment (`mise.local.toml`, `.env`) and defaults to `"off"`
  (`keywords/mode.go`), so `ResolvingMetricsStore.resolveAll`
  (`extract-metrics.go`) has never received a live answer.
- **Correction to spec `2026080403` D9's characterization — verified live,
  not just by reading code (task 1, done during implementation).** The
  spec's status table describes `"on"` mode as "unusable: no consumer
  exists, and `IsObserveMode()` means flipping to `on` turns collection off
  (K7)." Tested directly against real `miner` data: `"observe"` and `"on"`
  behave **identically** in every call tested (`ResolveName` and
  `ResolveAndObserve` alike) — the K7 story as literally described does not
  reproduce. **What does reproduce, found by testing the write path
  directly:** `Resolver.ResolveAndObserve` (`ontology/names/resolver.go`)
  discarded the `*semid.Resolution` returned by
  `KeywordFamily.ObserveOccurrence` (`_, err := r.Family.ObserveOccurrence(...)`).
  `ObserveOccurrence` auto-creates a provisional concept on a targeted
  deferred/human_review miss (D11) and sets that concept's id on its own
  return value (`ResolvedNodeID`) — but because the caller threw that value
  away, the concept was written to `kb.keyword_concepts` (confirmed:
  `status='provisional'`, `gloss_source='auto:d11'`) while
  `ResolveAndObserve`'s returned `NameResolution` still reported
  `status=unresolved`, `concept_id=""`. **Fixed as part of this task**
  (`resolver.go`): the discarded return value is now captured and used to
  fill in `res.ConceptID`/`res.Status`/`res.Method`/`res.Confidence`
  whenever the read pass (`ResolveName`) left them empty — never overriding
  an existing term/concept hit. Verified live post-fix: the same test case
  now returns `status=lexical_resolved` with the real `concept_id`. No
  existing test broke (`go test ./server/api/ontology/names/...` clean); the
  scenario had no prior test coverage, which is consistent with this being a
  real, previously-unnoticed gap rather than an intentional contract this
  fix violates.
- **Separately, a second real gap, also found live:** `ResolvingMetricsStore.
  resolveAll` (`extract-metrics.go`) calls the read-only `Resolver.ResolveNames`
  (→ `ResolveName`), which **by design never auto-creates** — D11
  auto-creation is exclusively a write-path (`ObserveName`/`ResolveAndObserve`)
  effect, per the code's own comment ("auto-creation is a write and belongs
  to the observe calls"). So even with the `ResolveAndObserve` bug fixed and
  `KEYWORD_RESOLVER_MODE` on, `resolveAll` still cannot get a `ConceptID` for
  a genuinely new metric name — it never calls anything that writes. **D4
  (below) is revised accordingly:** the metrics-persistence seam must call
  `ResolveAndObserve` (occurrence-aware), not `ResolveNames`.
- The only existing write path to `kb.ontology_terms`
  (`CandidateStore.PromoteToContent`) requires `status == 'approved'`,
  reached only via the human review-API state machine.
- Auto-alignment to an *already-released* term already exists and is fully
  automatic: `AlignmentsStore.EnsureAccepted` (`Actor: "auto-align"`) and
  `Resolver.ResolveAndObserve`'s pref-label auto-assign (`resolver.go:246-274`)
  both link a concept to an *existing* released `metric_definition` term
  with zero human involvement. **What's missing is only the create-if-absent
  step** — today, if no released term matches, `TermID` simply stays empty
  forever.

## Goals / Non-Goals

**Goals:**
- Every metric that resolves to a `kb.keyword_concepts` row also resolves to
  a `kb.ontology_terms` `metric_definition` row — creating one automatically
  when no aligned term exists yet.
- The created term is usable immediately by consumers (notably the
  Document Review comparison matrix), not blocked behind human review.
- Auto-created terms remain distinguishable from curator-reviewed ones for
  future sampling/exception-repair, without that distinction gating usage.
- `extract_metric_definitions` stops being load-bearing for whether a metric
  has a definition, and is removed from the default pipeline without
  deleting it.

**Non-Goals:**
- Building a review/sampling UI for auto-promoted terms (ADR §5 OD4 — future
  work).
- Reconciliation/merge support for duplicate auto-promoted terms (ADR §5
  OD2 — future work; today's `cmd/keyword-reconcile` only merges concepts).
- Redesigning the keyword module's tiers 0–6 — this change is a pure
  consumer of `kb.keyword_concepts`, not a modification to how concepts are
  matched.
- Repurposing `extract_metric_definitions`'s output as a `definition`-field
  enrichment source for auto-promoted terms (ADR §5 OD3 — plausible
  follow-on, not designed here).

## Decisions

### D1. Concept→term resolution is a separate, non-fuzzy step from name→concept resolution

Add a new method, `AlignmentsStore.EnsureAcceptedOrCreate(ctx, conceptID,
metricFields, method, score, evidence)`, alongside the existing
`EnsureAccepted`. Behavior:

1. Check for an existing accepted `aligns_to_term` for `conceptID`
   (`AcceptedForConcept`, already built) — if found, return it unchanged
   (identical to `EnsureAccepted`'s first branch).
2. If none exists, build a new `kb.ontology_terms` row
   (`term_kind='metric_definition'`, `status='auto-promoted'`, see D2/D3)
   from the concept and the triggering metric's fields, insert it via
   `terms.TermStore.CreateTerm` (existing store, currently only called from
   `candidates/promote.go`'s `promoteTerm` — this is a second caller, not a
   new store), then create the `aligns_to_term` assertion pointing to it
   (same `assertions.CreateAssertion` call `EnsureAccepted` already makes).
3. Steps 2's term-insert and alignment-insert happen in one transaction —
   a term with no alignment, or an alignment to a nonexistent term, must
   never be observable.

No tier 0–6 matching runs in this step. A `concept_id` is already a
deduplicated identity by construction (that's the keyword module's whole
job); the only question left is existence, not similarity.

**Alternative considered:** run the same tiered fuzzy match against
*existing* term labels before creating a new one (in case two concepts that
should have merged both reach this step independently). Rejected for this
change: it would duplicate matching logic the keyword module already owns,
and the actual defense against that scenario is concept-level convergence
(tiers 0–6 already ran before this step), not a second lexical pass at the
term layer. If duplicate auto-promoted terms turn out to be common in
practice, that's evidence tier 0–6 needs tuning, not that this step needs
its own matcher (ADR DR5 — measure, then decide).

### D2. `kb.ontology_terms.status = 'auto-promoted'`

Migration: extend `ontology_terms_status_check` from `(draft, in_review,
approved, included_in_release, superseded, rejected)` to add
`auto-promoted`. Placed conceptually alongside `included_in_release` (both
are "live" states) but kept distinct so `create_by`/`modify_by` and the
status itself make auto-created rows sampleable, matching D11's
attributability requirement extended from concepts to terms.

No transition *out* of `auto-promoted` is designed here (e.g. a curator
later "promoting" it to `included_in_release`) — out of scope per Non-Goals;
the status check constraint only needs to accept the new value, not enforce
a new edge in whatever reviews it later.

### D3. Term payload synthesis: structural transcription, not generation

**Schema gap found and closed during implementation:** `kb.ontology_terms` had
no columns for `value_type`/`range_type`/`permitted_units` at all — ADR
`2026072901` §3.24 described them as part of a `metric_definition` term's
content, but `candidates/promote.go`'s `promoteTerm` never actually
persisted them (only `definition`/`scope` survive candidate promotion
today). Added via migration `20260812000002` (`value_type TEXT`,
`range_type TEXT`, `permitted_unit_term_ids JSONB`), additive/nullable, no
change to existing callers' behavior.

`EnsureAcceptedOrCreate`'s caller (the `ResolvingMetricsStore` seam, see D4)
passes a small struct built from the triggering `kb.metrics` row and the
resolved concept — no LLM call. Field mapping is exactly ADR §DR3's table.
Unit resolution (`metric_unit` → a QUDT unit term) reuses whatever
existing unit-lookup path `associate_semantics`/`resolveUnitTerms` already
has (`extract-metrics.go` references `canonicalUnitForm`/
`unitQuantityKindMap` — spec `2026080403` §17.2 already flags these as
workaround maps, not a pattern to copy, but they're the only existing
unit-resolution logic and reusing them read-only here doesn't deepen that
coupling). If unit resolution misses, `permitted_units` is left empty rather
than guessed — same discipline `extract_metric_definitions`'s prompt already
used.

### D4. Call site: extend `ResolvingMetricsStore`, switch it to the write-aware resolve call

`ResolvingMetricsStore.resolveAll` (`extract-metrics.go:193`) currently
calls `r.Resolver.ResolveNames` (read-only) and sets
`metric_definition_term_id` only `if res.Status ==
names.StatusTermResolved`. Two changes, per the corrected findings above:

1. **Switch from `ResolveNames` to a per-name `ResolveAndObserve` call.**
   `ResolveNames` never auto-creates a concept (it's the read-only path);
   without this switch, D1's "auto-create a concept, then a term" chain
   never starts for a name seen for the first time — which, in practice, is
   almost every metric name (`kb.keyword_concepts` currently has zero
   document-sourced concepts). This does mean `resolveAll` now writes an
   occurrence per metric name (via `ObserveOccurrence`) where it previously
   wrote nothing — an intended, not incidental, consequence: D11's
   attributable/sampleable requirement (design.md Context) depends on that
   occurrence existing.
2. When the resulting status is `StatusLexicalResolved` or
   `StatusAmbiguous` (a concept was assigned but no term matched yet) and
   `ConceptID != ""`, call `EnsureAcceptedOrCreate` (D1) before falling back
   to leaving `metric_definition_term_id` empty.

`NameOccurrence.ArtifactType`/`ArtifactID`/`FieldPath` should identify the
triggering `kb.metrics` row (e.g. `"metric"`, the row's eventual id or a
stable pre-insert key, `"metric_name"`), mirroring the shape
`extract-test-methods.go`/other occurrence-writing call sites already use,
if any exist — confirm during implementation.

This keeps the single decorator seam ADR `2026072901` §16.3 already
established — no new write path beyond what `ResolveAndObserve` itself
performs, no risk of the two-writer divergence D3 in spec `2026080403`
explicitly fixed for normalizers.

### D5. `ComparisonStore.validateMetricKey` accepts `auto-promoted`

One-line change to the accepted status set. No change to `metric_key`
*being* the term id (ADR `2026072901` §2.4/REQ-4 stands) — only which term
statuses are eligible.

### D6. Retiring `extract_metric_definitions`

Remove `"extract_metric_definitions"` from wherever it's selected by
default (pipeline-policy binding / `config.toml`'s `required_processors`
equivalent for routed processors — confirm exact mechanism during task
execution, since routed processors are policy-selected, not
`config.toml`-listed, per capsule §7.1). Leave `NewMetricDefinitionsProcessor`,
its registration in `main.go`, and its prompt untouched, so it remains
callable via Dev Mode / explicit `operation` selection if anyone wants to
run it manually later. Update the capsule doc and `+CAPSULE.md` per ADR
DR6.

## Risks / Trade-offs

- **[Risk] Fragmented auto-promoted terms** (two terms for one real metric,
  if concept-level convergence is imperfect) **→ Mitigation:** none built in
  this change beyond what already exists at the concept layer (tiers 0–6 +
  offline reconciliation merge). Explicitly accepted per ADR DR5 as a
  monitored, non-blocking risk; `status='auto-promoted'` is what makes it
  findable later. Task list includes the pre-rollout observe-mode sample
  check ADR DR5 calls for.
- **[Risk] `KEYWORD_RESOLVER_MODE` behavior doesn't match this design's
  assumption once actually tested** (the K7 correction above is based on a
  code read, not a live-DB test yet) **→ Mitigation:** Task 1 runs that test
  before any other task depends on its outcome.
- **[Risk] Term auto-creation races** (two concurrent metric-save calls for
  the same never-before-seen concept both find "no alignment" and both try
  to create a term) **→ Mitigation:** the transactional insert-then-align in
  D1 step 3, plus relying on `AcceptedForConcept`'s existing conflict gate
  (`ErrAlignmentConflict`) to make a second writer's alignment attempt fail
  closed rather than silently create a second term; needs a concurrency
  test, not just a happy-path one.
- **[Trade-off] Comparison-matrix rows now include unreviewed content.**
  Explicit, intended consequence of ADR `2026081201`, not a side effect —
  documented here so implementation doesn't try to "fix" it by adding a
  review gate back in.

## Migration Plan

1. Goose migration: add `'auto-promoted'` to `ontology_terms_status_check`.
2. Ship D1–D5 code changes behind no new flag — `KEYWORD_RESOLVER_MODE`
   itself remains the activation switch (already exists; this change doesn't
   add a second one). Deploying with the mode still `off` is a safe no-op.
3. Turn `KEYWORD_RESOLVER_MODE` on in a non-production/staging pass first
   (per DR5's observe-mode sample check) before relying on it for real
   Document Review comparisons.
4. Remove `extract_metric_definitions` from default routed selection last,
   after 1–3 are verified working, so there's no window where metrics get
   neither an explicit-definition candidate nor an auto-promoted term.
5. **Rollback:** disabling is `KEYWORD_RESOLVER_MODE=off` (stops new
   auto-creation immediately) — no data migration needed to roll back, since
   `auto-promoted` rows are additive and nothing else depends on their
   absence. Re-adding `extract_metric_definitions` to the default selection
   is a config revert.

## Open Questions

- Does the K7 characterization in spec `2026080403` hold once tested live,
  or was it already fixed / inaccurate? (Task 1.)
- Exact mechanism for "routed processor default selection" to remove
  `extract_metric_definitions` from (pipeline-policy binding vs. some other
  config surface) — capsule §7.1 says routed processors need "a resolved
  pipeline policy," not a `config.toml` list; confirm against
  `pipeline_bindings.go`/`policy_seed.go` during task execution.
