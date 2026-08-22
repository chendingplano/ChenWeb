## Context

Today `resolveOrCreateMetricClass` (`server/api/ontology/assertions/metric_lossless_writer.go:242`)
picks a metric's class from exactly one of, in order: a prior `MetricDefinitionTermID`, keyword-
concept alignment (`ConceptID` → `AlignmentsStore.AcceptedForConcept`), or a hash of the raw
`metric_name` string. None consult `subject`/`object_name`/`value_class`, so two metrics with the
same name but different subjects collapse into one class on the primary (concept) path.

`kb.ontology_terms.properties` (JSONB, added by migration `20260822000001`) is populated today only
by `TermSynthesisInput.termProperties()` (`server/api/ontology/keywords/alignment.go:313`): fixed
`value_type`/`range_type`/`raw_unit`/`permitted_unit_term_ids` plus whatever
`[ontology_term_property_map]` exposes as flat `"artifact_type:field:property"` strings
(`server/api/ontology/assertions/property_map.go`, `ParsePropertyMap`/`BuildMappedProperties`) — an
unstructured bag with no resolve-against-a-catalog behavior for anything except unit
(`resolveUnitTerms`, `associate_semantics.go:538`).

The one existing precedent for "raw value, resolve against a governed catalog, keep the raw value
regardless, propose if unresolved" beyond units is `kb.metric_value_range_type_map`
(`ValueRangeTypeMapper`, `metric_normalizer.go:460`): a DB-backed, in-process-cached, TTL'd,
invalidate-on-write lookup table. It is looked up exactly once per metric row, in
`MetricNormalizer.Normalize` (the `normalize_assertions` stage), and the *result* — not a second
lookup — flows through `DecisionCandidate.ProposedPayload` into `metricCandidatePayload` and is read
by `writeMetricLossless` later. This idempotency pattern (resolve once at propose time, read
downstream) is the one this design reuses for every new identity dimension, rather than inventing a
second resolve point.

`AssociateSemantics.writeMetricLossless` runs inside one transaction per metric occurrence
(`metric_lossless_writer.go:52`) and already calls `classfoundation.ClassResolutionDecisionStore.
RecordIfChanged` (append-only decision history) with a hardcoded `nil` alternatives argument, and
already has a `semantic.ClassAmbiguousCandidates` identity-state constant wired all the way through
`recordMetricStageOutcomes`'s finding emission (`FindingClassAmbiguous`, `SeverityWarning`) — but
`resolveOrCreateMetricClass` never actually produces that state today. A separate, older
`classfoundation.ClassResolutionService` (`class_resolution_service.go`) already demonstrates the
intended shape for an ambiguous outcome — create/select a **new** provisional class rather than
guess between tied candidates, record the tied candidates as alternatives, still never return
classless — but it matches candidates on `QuantityKindTermID` equality against caller-supplied
`Safe`-flagged candidates, a narrower, different-shaped contract than DR3's per-dimension governed
signature. This design does not reuse it directly (see Decisions), but matches its externally
visible contract for the ambiguous case, since `class-resolution-decisions` spec already requires it
("the system SHALL persist the ambiguity and its alternatives" / "no safe match creates a
provisional class... rather than discard the metric or attach it classlessly" — both already-existing
requirements, not new ones this design introduces).

**Verified gaps that shape scope** (see proposal.md's Impact section for detail): ADR `2026082102`
(hybrid search, DR6's dependency) has zero implementation in this repo — no table, no code, no
openspec change. `config.local.toml` (gitignored, per-environment) does not currently match what the
in-progress, uncommitted `fix-metric-property-scope-conflation` change's `tasks.md` claims it does —
irrelevant to this design's correctness since DR4 rewrites the section wholesale, but noted so the
migration step doesn't assume a starting shape it should instead just overwrite.

## Goals / Non-Goals

**Goals:**
- Generalize the value-range-type resolve-or-propose pattern into one table
  (`kb.governed_property_value_map`) usable by any occurrence field, with zero opinion on which
  governed vocabulary (`term_kind`) an approved value belongs to (OD2 stays a curator/data decision,
  not a code decision — see Decisions).
- Make `kb.ontology_terms.properties` a deterministic class-identity signature for `identity = true`
  configured fields, distinguishing "unresolved" (`term_id: null`) from "not configured" (key
  absent).
- Give `resolveOrCreateMetricClass` a real signature-matching step, strictly conservative (agreement
  required, never a guessed merge), before its existing three-step fallback — behavior is provably
  unchanged (falls through every time) until `kb.governed_property_value_map` has approved rows.
- Migrate `[ontology_term_property_map]` to the table-array shape DR4 specifies, resolving OD4.

**Non-Goals:**
- DR6 / `search_document` integration — no dependency exists yet (see Context).
- Picking `term_kind`s for governed dimension values, or writing any code that mints them (OD2) —
  approval stays a manual/curator action against the new table, mirroring
  `kb.metric_value_range_type_map`'s existing operational shape exactly.
- Stronger per-dimension normalization than the existing lowercase/trim/dash-underscore floor (OD1)
  — deferred until real `proposed`-row volume exists to inform it, per the ADR's own stated
  precondition.
- Merging/renaming `object_literal`↔`qualifiers` (OD5) — two columns stay, unchanged shape.
- A scoring/ranking model for partial signature matches beyond "most shared-agreeing dimensions,
  tie → ambiguous" (see Decisions) — full corpus-informed tuning is future work per OD3's own text.

## Decisions

### D1 — One resolver type, one table, dimension as a plain string column (not an enum, not a `term_kind`)

`kb.governed_property_value_map` is exactly the ADR's DR3 DDL: `PRIMARY KEY (dimension, raw_value)`,
`term_id` nullable (`REFERENCES kb.ontology_term_headers(term_id)`, no `ON DELETE` action — matches how
other term-referencing FKs in this schema behave, e.g. `unit_term_id` on assertions), `status`
CHECK'd to `proposed|approved|ambiguous|rejected`, occurrence/first-seen/last-seen bookkeeping. A new
`GovernedPropertyResolver` (new file, `server/api/ontology/assertions/governed_property_resolver.go`)
mirrors `ValueRangeTypeMapper` structurally: same TTL'd whole-table in-process cache (keyed by
`dimension + "\x00" + raw_value` since the cache now spans multiple dimensions, unlike
`ValueRangeTypeMapper`'s single-dimension table), same invalidate-on-write, same
`Lookup(ctx, dimension, raw, recordID) (termID, status string, err error)` contract returning `("",
"proposed", nil)` on a fresh miss after auto-inserting it. `normalize(raw_value)` is
`normalizeValueRangeTypeRaw`'s exact existing rule (OD1 floor), reused via the already-exported
`NormalizeValueRangeTypeRaw` rather than duplicated.

**Rejected: per-dimension tables.** Would repeat `kb.metric_value_range_type_map`'s shape N times
(exactly what §2.3 of the ADR calls out as the problem being fixed) and would need N cache
instances, N admin-handler variants. One table with `dimension` as a discriminator column costs one
extra `WHERE`/cache-key segment and buys a single triage surface for every dimension.

### D2 — Identity-dimension resolution happens once, at `normalize_assertions` time, not at write time

Matches `ValueRangeTypeMapper.Lookup`'s existing call site exactly: `MetricNormalizer.Normalize`
(`metric_normalizer.go:128`) already resolves `value_range_type` once per row and threads the result
through `DecisionCandidate.ProposedPayload` for `writeMetricLossless` to read later, never
re-resolving (the existing comment on `resolveMetricMappingState` explains why: re-resolving would
double-count `occurrence_count` on a retry). The same `Normalize` loop gains one
`GovernedPropertyResolver.Lookup` call per `identity = true` configured field (typically 3-4:
`metric_name`, `metric_subject`, `object_name`, `value_class`), writing each field into
`class_properties` in DR2's `{"raw": ..., "term_id": ...}` shape instead of today's plain-value
`BuildMappedProperties` output. Non-identity `[ontology_term_property_map]` entries keep the plain
shape unchanged. `writeMetricLossless`/`resolveOrCreateMetricClass` read the already-resolved
signature out of `ClassSynthesisInput.ExtraProperties` — no new DB round trip inside the write
transaction beyond the new matching query itself (Decision D3).

**Rejected: resolving inside `writeMetricLossless`'s transaction.** Would duplicate the "resolve
once, read downstream" idempotency the codebase already establishes for the mapping dimension, and
would mean a retried write transaction re-touches `occurrence_count` — the same bug class the
existing comment on `resolveMetricMappingState` explicitly avoids.

### D3 — Strict signature matching: candidate classes ranked by shared-agreeing-dimension count; zero agreement is not a match; a tie is `ClassAmbiguousCandidates`, not a guess

Confirmed with the user (OD3): strict agreement on every dimension resolved on **both** sides. Two
gaps the ADR's own DR5 text leaves implicit, resolved here:

1. **Vacuous match:** "matches on every dimension resolved on both sides" is vacuously true for zero
   shared dimensions — every class in the catalog would "match" a wholly-unresolved occurrence.
   Resolved: a class must share **at least one** dimension where *both* sides resolved to the same
   non-null `term_id` to count as a candidate at all. Zero shared-agreeing dimensions = no match,
   falls through to the existing fallback exactly as today.
2. **Multiple non-contradicting candidates:** two classes can each satisfy "no contradiction" while
   agreeing on different, non-overlapping dimensions (class A agrees only on `subject`, class B only
   on `value_class`) — neither contradicts the occurrence, so "no contradiction" alone under-
   constrains. Resolved: rank qualifying candidates by **count** of shared-agreeing dimensions:
   - Exactly one candidate has the max count (≥1) → reuse it, `ClassResolvedExisting`.
   - ≥2 candidates tie at the max count → do **not** guess between them. Fall through to the
     existing create-path (`SynthesizeClass`, unchanged) to mint a **new** provisional class, but set
     `identityState = semantic.ClassAmbiguousCandidates` instead of `ClassProvisionalNew`, and pass
     the tied candidates to `ClassResolutionDecisionStore.RecordIfChanged`'s alternatives parameter
     (currently hardcoded `nil` — this is the first call site to populate it). This exactly matches
     `class-resolution-decisions`' existing, previously-unexercised requirements ("ambiguous class...
     alternatives... without selecting an authoritative [*existing*] class" / "no safe match creates
     a provisional class... never classless") and mirrors `ClassResolutionService.Resolve`'s already-
     established ambiguous-still-provisions behavior, without adopting that service's narrower
     quantity-kind-only candidate contract.

Query shape: against `kb.ontology_terms_current` (the compatibility view), filtered to the
occurrence's `term_kind` (`metric_definition` for this family) and to usable-content status
(`included_in_release` or `auto-promoted`, matching `releasedTermExistsSQL`'s existing definition of
"usable"), with a `WHERE` clause of `(properties->dim->>'term_id' IS NULL OR properties->dim->>
'term_id' = $resolved_term_id)` ANDed across every dimension the occurrence resolved (the "no
contradiction" filter) — then Go-side counts, per returned row, how many of those dimensions are
non-NULL-and-equal (the "agreement" count, always ≤ the WHERE-filtered set), and applies the ranking
above. No new index is added in this pass (candidate catalogs are small — metric_definition terms,
not a corpus-wide scan); a GIN index on `properties` is a documented future optimization if profiling
shows it's needed.

**Rejected: minimum-agreement threshold or staged rollout** (the ADR's other two OD3 options) — user
confirmed strict-agreement-with-ambiguity-fallback as the safer default; matches the ADR's own
repeated framing that false merges are the failure mode this whole ADR exists to stop, and a
zero-corpus-measurement table is not a defensible basis for tuning a threshold today.

### D4 — `[ontology_term_property_map]` becomes `map[string][]OntologyTermPropertyMapEntry`, keyed directly by artifact type; `[semantic_assertion_property_map]` is untouched

TOML's `[[ontology_term_property_map.metric]]` nests directly under the `ontology_term_property_map`
key, so the Go config type becomes `map[string][]OntologyTermPropertyMapEntry` (entry: `Field`,
`Property`, `Identity bool`, `Dimension string`) — no wrapper struct with a `PropertyMap []string`
field, and no `ParsePropertyMap`-style string-splitting for this section any more; `mapstructure`
decodes the array-of-tables directly into typed entries. `[semantic_assertion_property_map]` keeps
its current `PropertyMap []string` / `"artifact_type:field:property"` shape and its existing
`ParsePropertyMap`/`BuildMappedProperties` code path unchanged — DR4 is explicit that instance-level
fields are unaffected.

Per DR4's own validation ask: at config-load time, if `AppConfig.OntologyTermPropertyMap["metric"]`
(the one `term_kind` this pass wires signature resolution for) has zero `Identity == true` entries,
`LoadConfig` logs a **warning** (not a hard error) via the existing logger, matching the ADR's
explicit "warning, not hard error" instruction.

**Rejected: keep the flat string format and bolt on identity/dimension via a 4th colon-separated
segment.** The ADR's own DR4 text rules this out ("cannot carry a third/fourth attribute cleanly");
`mapstructure` already decodes table arrays elsewhere in this config, so there's no new parsing
mechanism to build.

### D5 — Migration: two files, additive-only to code, breaking-but-mechanical to config

1. New goose migration `project_migrations/<ts>_create_kb_governed_property_value_map.sql` — DDL
   only (`CREATE TABLE IF NOT EXISTS` + status index), matching D1. No backfill: unlike
   `kb.metric_value_range_type_map`'s DR5 seed (which preserved an existing hardcoded Go switch's
   behavior), there is no prior classification behavior for `metric_subject`/`object_name`/
   `value_class`/`metric_name` resolution to preserve — every value starts `proposed`, which is
   correct (matches "informed by real proposed volume after DR3 ships" — OD1/OD2's own stated
   precondition; there is no real volume before this ships).
2. Config: rewrite `[ontology_term_property_map]` in `config.toml`/`config.local.toml` to the new
   table-array shape, with `identity = true` + `dimension = "metric_<field>"` on exactly the four
   fields DR4's own example lists (`metric_name`, `metric_subject`→`subject`, `ext_info.object_name`→
   `object_name`, `value_class`), everything else from today's working-tree state (12 flat entries,
   findings from the proposal's Impact section) reclassified: fields that are genuinely class-level
   but non-identity keep `identity = false` (no `dimension`); fields that are instance-level move to
   `[semantic_assertion_property_map]` instead (unchanged flat format) if not already covered by
   `fix-metric-property-scope-conflation`'s in-flight, uncommitted work once that lands.

Rollback: `DROP TABLE IF EXISTS kb.governed_property_value_map` (goose Down); config changes are
edited back by hand like any other config edit (config files are not goose-migrated).

## Risks / Trade-offs

- **[Risk] Every dimension starts 100% `proposed`** → DR5's signature step never finds a candidate
  for a long time, so behavior is identical to pre-ADR fallback until curators triage.
  **Mitigation:** this is explicitly the intended, safe rollout shape (D3/D5) — no behavior
  regression risk, only a "benefit arrives gradually" characteristic already called out in the ADR's
  own §4.2.
- **[Risk] `governed_property_value_map` becomes a permanent unattended queue** (ADR §4.2, same
  operational risk `kb.metric_value_range_type_map` already carries). **Mitigation:** none new in
  this design; an admin triage UI/handler mirroring `metric_range_type_errors_handler.go` is a
  natural, separately-scoped follow-up, not required for this ADR's own decisions to be correct.
- **[Risk] Signature-candidate query cost grows with catalog size** (a full `properties` JSONB scan
  per occurrence). **Mitigation:** deferred (see D3) — `metric_definition` term counts are currently
  small; add a GIN index if profiling later shows otherwise.
- **[Trade-off] Ambiguity ranking (D3) is a design decision beyond what the ADR's text states
  verbatim**, since the ADR's own ranking rule as written is vacuous for the zero-overlap case. Flagged
  explicitly here rather than silently resolved, so a future reader can see this was a deliberate
  fill-in, not an oversight.

## Migration Plan

1. Land `kb.governed_property_value_map` migration; verify via `mise dev`'s goose auto-apply
   (matches this workspace's live-reload behavior — see project memory on ChenWeb's openspec
   workflow) that it applies cleanly against the dev DB.
2. Land the Go resolver + config restructuring + `Normalize`/`resolveOrCreateMetricClass` changes
   together (they are not independently deployable — the config shape change and the Go struct that
   reads it must land atomically).
3. Rewrite `config.local.toml`'s `[ontology_term_property_map]` to the new shape as part of the same
   change (see D5 point 2).
4. No production data migration or backfill needed (D5) — every dimension starts `proposed`, and
   `resolveOrCreateMetricClass`'s new step is a pure no-op addition until curators approve rows.

## Open Questions

- Whether the eventual admin triage surface for `kb.governed_property_value_map` reuses
  `metric_range_type_errors_handler.go`'s pattern verbatim or needs its own multi-dimension-aware
  variant — out of scope for this change; only the table and resolver are built here.
- OD1 (normalization strength) and OD2 (term_kind ownership) remain genuinely open, deferred exactly
  as the ADR itself specifies, pending real `proposed`-row corpus volume this change is what
  produces.
