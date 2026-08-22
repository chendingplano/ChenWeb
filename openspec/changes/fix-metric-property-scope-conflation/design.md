## Context

Bug 2026082101's round-2 review agreed four problems need fixing:

1. `kb.ontology_terms` classes for metrics are minted inside
   `extract-metrics.go`'s `resolveAll` (`s.Alignments.EnsureAcceptedOrCreate`,
   `extract-metrics.go:284`) — before `normalize_assertions`/
   `associate_semantics` ever runs. Class creation should be triggered by
   `associate_semantics` instead.
2. `kb.ontology_terms.properties` mixes class-level facts with per-occurrence
   instance data (`metric_subject`, `value_min`/`value_max`, `condition`, ...)
   sourced from whichever metric row happened to trigger promotion first.
3. Property key names aren't normalized: the config map's
   `value_data_type`/`value_range_type` entries duplicate the fixed
   synthesis's `value_type`/`range_type` under different names on the same
   term.
4. `kb.semantic_assertions.qualifiers` is the right home for instance-level
   metric data (already designed for this — `extract-metrics-structured-output/
   design.md:177`) but is populated by a hardcoded 3-field function
   (`metricQualifiers`, `associate_semantics.go:323-332`) instead of a
   configured map.

Tracing (1) surfaced two facts that shape this design:

- **Import cycle**: `keywords` (home of `EnsureAcceptedOrCreate`) imports
  `assertions` (home of `associate_semantics`/`metric_lossless_writer.go`) to
  persist keyword-concept↔term alignments as `kb.semantic_assertions` rows.
  `assertions` can never import `keywords` back.
- **Undiscovered visibility bug**: `associate_semantics`'s own class path
  (`metric_lossless_writer.go:222` `resolveOrCreateMetricClass` →
  `classfoundation.ContractStore.CreateIdentityOnlyClass`) inserts only into
  `kb.ontology_term_headers` and `kb.ontology_class_contract_revisions` — never
  `kb.ontology_terms`. Because `kb.ontology_term_revisions.source_term_row_id`
  is `NOT NULL REFERENCES kb.ontology_terms(id)`, that path structurally
  cannot produce a `kb.ontology_terms_current` row. Every provisional class
  minted this way today is invisible to the normal term catalog. Moving class
  creation into `associate_semantics` must fix this as a precondition, or it
  makes every class invisible instead of just the fallback ones.

```
BEFORE                                              AFTER

extract_metrics ──EnsureAcceptedOrCreate──►         extract_metrics ──ResolveAndObserve──►
  kb.ontology_terms (real, visible)                   kb.metrics.keyword_concept_id only
        │ writes metric_definition_term_id                   │
        ▼ back onto kb.metrics (sometimes)                   ▼
normalize_assertions ──► candidate payload           normalize_assertions ──► candidate payload
  (metric_definition_term_id, if set)                   (keyword_concept_id, + qualifiers fields)
        │                                                     │
        ▼                                                     ▼
associate_semantics ──CreateIdentityOnlyClass──►      associate_semantics ──ClassSynthesizer──►
  headers-only row (NOT in kb.ontology_terms_current    kb.ontology_terms (real, visible) via
  when alignment hadn't already run)                    keywords' registered implementation
```

## Goals / Non-Goals

**Goals:**
- Exactly one trigger point creates/selects a metric's `kb.ontology_terms`
  class row: `associate_semantics`, at the point a metric occurrence actually
  becomes a represented/accepted assertion.
- Every class `associate_semantics` creates or selects is a real,
  `kb.ontology_terms_current`-visible row — including today's headers-only
  fallback case.
- `kb.ontology_terms.properties` for `metric_definition` terms holds only
  class-level facts; no property fact is duplicated under two key names.
- `kb.semantic_assertions.qualifiers` holds instance-level metric facts,
  populated from a configured map with the same shape as
  `[ontology_term_property_map]`.
- The keyword-concept↔term alignment link (curator-facing, from
  `auto-promoted-governed-terms`) keeps working when a `keyword_concept_id`
  is available.

**Non-Goals:**
- Not touching `classfoundation`'s contract-revision/capability-validation
  bookkeeping (`kb.ontology_class_contract_revisions`, `DefinitionState`).
  The legacy `EnsureAcceptedOrCreate` path has never populated it for
  `metric_definition` terms either; this change doesn't change that.
  Wiring class synthesis into capability validation is separate future work.
- Not extending either property map to `provisions`/`entity`/`relation`
  (already out of scope per the original bug's follow-up).
- Not backfilling existing `kb.ontology_terms`/`kb.semantic_assertions` rows.
  Per the original bug's resolution direction, affected data is cleared via
  `scripts/clear-artifact-data.sql`, not migrated.
- Not changing `kb.ontology_terms_current`'s view definition or the
  `sync_ontology_term_revision_after_insert` trigger — the fix routes through
  the existing `kb.ontology_terms` writer (`terms.TermStore.CreateTerm`)
  rather than around it.

## Decisions

### D1: `ClassSynthesizer` registered seam in `assertions`, implemented in `keywords`

Add to `assertions` (new file, mirroring `normalizer_registry.go`'s existing
`Normalizer`/`RegisterNormalizer` shape):

```go
type ClassSynthesisInput struct {
    ConceptID            string // keyword_concept_id; may be empty
    CanonicalName        string
    Definition            string
    ValueType             string
    RangeType             string
    PermittedUnitTermIDs  []string
    RawUnit               string
    ExtraProperties       map[string]any // class-level only
}

type ClassSynthesizer func(ctx context.Context, db DBX, candidateTermID string, input ClassSynthesisInput) (termID string, created bool, err error)

func RegisterClassSynthesizer(fn ClassSynthesizer)
func SynthesizeClass(ctx context.Context, db DBX, candidateTermID string, input ClassSynthesisInput) (termID string, created bool, err error)
```

`db DBX` (assertions' existing `*sql.DB`/`*sql.Tx`-compatible interface,
`assertions_store.go:83`) lets `metric_lossless_writer.go` call this inside
its existing transaction, preserving DR5's atomic-write guarantee.

`keywords` registers the implementation from its own `init()` (valid
direction: `keywords` already imports `assertions`). The implementation reuses
`EnsureAcceptedOrCreate`'s term-creation logic against the caller's `db`:
when `ConceptID != ""` it runs the same existing-alignment-check +
`terms.TermStore.CreateTerm` + alignment-assertion write `EnsureAcceptedOrCreate`
does today; when `ConceptID == ""` (concept resolution didn't run or found
nothing) it falls back to a bare `terms.TermStore.CreateTerm` with no
alignment link.

**Alternative considered**: have `metric_lossless_writer.go` call
`keywords.AlignmentsStore.EnsureAcceptedOrCreate` directly. Rejected — import
cycle, not fixable without restructuring package boundaries far beyond this
change's scope.

**Alternative considered**: don't preserve the concept↔term alignment link;
have `associate_semantics` create classes independently via `terms.TermStore`
directly, dropping `keywords` involvement entirely. Rejected per your
confirmed direction — this silently drops the curator-facing keyword-catalog
feature (`auto-promoted-governed-terms`, 27/27 tasks shipped).

### D2: `EnsureAcceptedOrCreate`'s core is extracted into a tx-scoped helper

`EnsureAcceptedOrCreate` currently opens its own transaction via
`withKeywordIdentityMutation(ctx, s.Assertions.DB, ...)`. The registered
`ClassSynthesizer` must run inside `metric_lossless_writer.go`'s existing
transaction instead of opening a second one. `alignment.go`'s
create-term-and-align logic (currently the body of that closure) is extracted
into an unexported, transaction-agnostic helper both the public
`EnsureAcceptedOrCreate` (wraps the helper in its own transaction, unchanged
external behavior) and the registered synthesizer (calls the helper with the
caller-supplied `db`) share. Existing `alignment_test.go` coverage must pass
unchanged — this is a behavior-preserving refactor of `EnsureAcceptedOrCreate`,
not a rewrite.

### D3: `extract-metrics.go` keeps concept resolution, drops term creation

`resolveAll` keeps calling `names.ResolveAndObserve` (dedup, `keyword_concept_id`
bookkeeping) but stops calling `EnsureAcceptedOrCreate`. `metric_definition_term_id`
is no longer set on `kb.metrics` at extraction time for a first-seen concept —
it's set once `associate_semantics` processes the occurrence. `keyword_concept_id`
is forwarded unconditionally (already computed by `ResolveAndObserve`
regardless of term-resolution status).

### D4: `metric_normalizer.go` forwards `keyword_concept_id` and the qualifiers source fields

`metricRow`/its `SELECT` gain `metric_subject`, `reasoning_tags`, `ext_info`,
`metric_categories`, `value_data_type` (needed by the new
`[semantic_assertion_property_map]`; `value_class`, `metric_value`,
`value_min`, `value_max`, `condition` are already selected).
`metricCandidatePayloadForRow` adds `keyword_concept_id` (from
`kb.metrics.keyword_concept_id`, already a column per `metrics_handler.go`)
and a `qualifiers` sub-object built by applying `[semantic_assertion_property_map]`
against the row's field map — same mechanism as D6, applied at normalize time
because that's where the raw field map already exists.

### D5: `resolveOrCreateMetricClass` calls the synthesizer, not `classfoundation`

`metric_lossless_writer.go` builds `ClassSynthesisInput` from
`metricCandidatePayload` (name, definition sourced the same way
`extract-metrics.go` sources it today — `metric_desc` falling back to
`formula_or_definition` — value_type/range_type/unit resolution unchanged)
and calls `assertions.SynthesizeClass`. `classIdentityState`
(`ClassResolvedExisting`/`ClassProvisionalNew`) is unchanged in meaning; the
existence check can stay against `kb.ontology_term_headers` (still populated,
now consistently, since `terms.TermStore.CreateTerm`'s insert trigger
maintains it) or move to `kb.ontology_terms_current` — implementation detail,
not a behavior change either way. `classfoundation.ContractStore.CreateIdentityOnlyClass`
becomes unused by the metric family (it has no other caller today); left in
place, not deleted, since `class_resolution_service.go`'s `ClassResolutionService`
still declares it as part of its `ClassCreator` interface for potential
future use.

### D6: `properties` scope and key normalization are config-only changes

No new code enforces "class-level only" — `[ontology_term_property_map]`'s
metric mapping is edited to remove the instance-level entries (`metric_subject`,
`threshold_or_target`, `measurement_frequency`, `value_min`, `value_max`,
`metric_value`, `condition`, `reasoning_tags`, `ext_info.object_name`,
`metric_categories`) and the two duplicate-key entries (`value_data_type`,
`value_range_type` — already covered by the fixed synthesis's `value_type`/
`range_type`). `formula_or_definition` is also removed: it's already carried
by `TermSynthesisInput.Definition` (as a fallback when `metric_desc` is
empty), so mapping it into `ExtraProperties` too duplicates it under a third
name (`properties.formula_or_definition` alongside `definition`) — the same
class of bug as finding 3, caught by applying its own logic one field further.
What's left mapped at class level: nothing beyond the fixed synthesis fields
today; the config section stays in place (and documented) for future
genuinely class-level fields, but starts empty for `metric`.

### D7: `[semantic_assertion_property_map]` reuses the property-map machinery, generalized

`ontology_term_property_map.go`'s parsing/lookup helpers
(`parseOntologyTermPropertyMap`, `buildOntologyTermProperties`,
`lookupFieldPath`, `isEmptyFieldValue`) are pure `map[string]any` utilities
with no ontology-term-specific logic. Renamed to generic names
(`parsePropertyMap`, `buildMappedProperties`) and reused by both config
sections rather than duplicated. `config.go` gains
`SemanticAssertionPropertyMapConfig`/`GetSemanticAssertionPropertyMap()`
mirroring the existing `OntologyTermPropertyMapConfig`.

### D8: `metricQualifiers` is removed, not extended

Per your feedback, `metric_definition_term_id` is dropped from qualifiers
entirely (redundant with `instance_of_term_id` once D1-D5 land) and
`metric_name`/`condition` become ordinary configured entries
(`metric:metric_name:metric_name`, `metric:condition:condition` in
`[semantic_assertion_property_map]`) rather than a hardcoded special case.
`associate_semantics.go`'s `processMetric` and `metric_lossless_writer.go`'s
`writeMetricLossless` both read the pre-built `qualifiers` map D4 attached to
the candidate payload directly as `Assertion.Qualifiers`.

## Risks / Trade-offs

- **[Refactor risk]** Extracting `EnsureAcceptedOrCreate`'s core (D2) could
  regress the existing keyword-alignment behavior. → Mitigation: the existing
  `alignment_test.go` suite exercises `EnsureAcceptedOrCreate`'s conflict
  gate, idempotency, and decision-log write; it must pass unchanged, proving
  the refactor preserves behavior rather than just compiling.
- **[Timing change]** `kb.metrics.metric_definition_term_id` is no longer set
  at extraction time (D3) — only once `associate_semantics`/Phase D runs. Any
  reader that queries it immediately after `extract_metrics` (before Phase D)
  will now see it empty where it was previously populated eagerly. →
  Mitigation: audit readers of this column during implementation (task list
  item); this is also the intended behavior (class creation should follow a
  real occurrence, not precede it), so a reader depending on the old timing
  needs adjusting, not preserving.
- **[Term-ID scheme shift]** Metrics that previously fell back to
  `measurement:auto:defname_<hash>` (no `metric_definition_term_id` yet) will
  now more often carry a concept-linked ID via the synthesizer's alignment
  path, since `keyword_concept_id` is now always forwarded. This is a
  behavior improvement (fewer orphaned hash-based classes) but changes which
  literal term_id a given metric resolves to compared to today's data. →
  Mitigation: no backfill needed (non-goal), consistent with the original
  bug's clear-and-reprocess stance for this staging environment.
- **[Config drift]** Removing `[ontology_term_property_map]` entries changes
  runtime output the moment the config deploys, with no versioning. →
  Mitigation: staging server, acceptable; called out explicitly here for the
  human editing `config.local.toml`.

## Migration Plan

No DB schema migration — `kb.semantic_assertions.qualifiers` and
`kb.ontology_terms.properties` already exist. Deploy is code + config:

1. Land D1/D2 (synthesizer seam + refactor) behind existing test coverage.
2. Land D5 (`metric_lossless_writer.go` wiring) — `classfoundation.CreateIdentityOnlyClass`
   stops being called for metrics.
3. Land D3/D4 (extract_metrics/metric_normalizer changes) together, since D4's
   payload shape is what D3's removal makes necessary.
4. Land D6/D7/D8 (config + qualifiers) last, since they depend on D4's fields
   being available.
5. Update `config.local.toml` per D6/D7.
6. Run `go test ./...` across the workspace; reprocess a sample input record
   through Phase D and inspect the resulting `kb.ontology_terms`/
   `kb.semantic_assertions` rows manually before calling this done.

Rollback: revert the commits; no schema to roll back.

## Open Questions

- Are there existing readers (UI, reports, other processors) of
  `kb.metrics.metric_definition_term_id` that assume it's populated
  immediately after `extract_metrics`, before Phase D runs? Not fully audited
  — flagged as a task-list item rather than resolved here.
