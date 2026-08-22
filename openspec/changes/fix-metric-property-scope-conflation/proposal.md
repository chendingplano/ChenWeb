## Why

Bug 2026082101's round-2 review found that `kb.ontology_terms` class terms for
metrics are created too early (inside `extract-metrics.go`, before any
`kb.semantic_assertions` occurrence exists) and that `kb.ontology_terms.properties`
mixes class-level facts with per-occurrence instance data, some of it under
duplicate key names. Tracing the fix surfaced two further, previously
undiscovered problems that make "just move the call" unsafe: `assertions`
(home of `associate_semantics`) cannot import `keywords` (home of the term
auto-promotion logic) without an import cycle, and `associate_semantics`'s own
class-resolution path (`classfoundation.CreateIdentityOnlyClass`) never writes
a `kb.ontology_terms` row at all — every provisional class it mints today is
already invisible to `kb.ontology_terms_current` and any reader that goes
through the normal term catalog. Fixing class-creation ownership requires
fixing that visibility gap first, not after.

## What Changes

- Add a `ClassSynthesizer` registration seam to the `assertions` package
  (mirrors the existing `RegisterAssociationResolver`/`RegisterNormalizer`
  self-registration pattern). `metric_lossless_writer.go`'s
  `resolveOrCreateMetricClass` calls the registered synthesizer instead of
  `classfoundation.ContractStore.CreateIdentityOnlyClass` when it needs to
  mint a genuinely new class.
- `keywords` registers the concrete synthesizer from its own `init()`
  (valid direction — `keywords` already imports `assertions`), reusing
  `EnsureAcceptedOrCreate`'s term-creation logic so the synthesized class is a
  real `kb.ontology_terms` row (visible via `kb.ontology_terms_current`) and
  the keyword-concept-to-term alignment link curators rely on is preserved.
- **BREAKING (internal)**: `extract-metrics.go`'s `resolveAll` stops calling
  `s.Alignments.EnsureAcceptedOrCreate` to create/promote a term. It keeps
  `names.ResolveAndObserve` only for `keyword_concept_id` dedup. Class
  creation now happens only when `associate_semantics` processes a metric
  occurrence into a represented/accepted assertion, not speculatively during
  extraction.
- `metric_normalizer.go` forwards `keyword_concept_id` and the additional raw
  `kb.metrics` fields needed by the new qualifiers map (below) into the
  `kb.semantic_decision_candidates.proposed_payload`.
- The auto-created term's `properties` payload is restricted to class-level
  facts only: `value_type`, `range_type`, `permitted_unit_term_ids`,
  `raw_unit` (unchanged), plus `formula_or_definition`-derived content.
  Instance-level fields (`metric_subject`, `threshold_or_target`,
  `measurement_frequency`, `value_min`, `value_max`, `metric_value`,
  `condition`, `reasoning_tags`, `object_name`, `metric_categories`) are
  removed from `[ontology_term_property_map]`'s metric mapping.
- Duplicate property key names are eliminated: `config.local.toml`'s
  `metric:value_data_type:value_data_type` and
  `metric:value_range_type:value_range_type` entries are removed from
  `[ontology_term_property_map]` since the fixed synthesis fields already
  cover the same facts under `value_type`/`range_type`.
- New `[semantic_assertion_property_map]` config section (same
  `<artifact_type>:<table_field_name>:<property_name>` shape as
  `[ontology_term_property_map]`) drives `kb.semantic_assertions.qualifiers`
  for the instance-level fields removed from the term-properties map above,
  replacing the hardcoded 3-field `metricQualifiers()`.

## Capabilities

### New Capabilities
- `metric-class-synthesis-seam`: the `ClassSynthesizer` registration seam in
  `assertions`, and `associate_semantics` owning the trigger point and content
  scope for metric `metric_definition` class creation (replacing both
  `extract_metrics`'s eager pre-normalize auto-promotion and
  `classfoundation`'s headers-only identity path for the metric family).
  Supersedes the intent of `governed-term-auto-promotion` and
  `ontology-term-kind-properties`, two capabilities from prior changes
  (`auto-promoted-governed-terms`, `fix-auto-promoted-term-schema-loss`)
  that are implemented in code but not yet archived to `openspec/specs/`,
  so this change cannot express its changes as deltas against them.
- `semantic-assertion-property-map`: config-driven population of
  `kb.semantic_assertions.qualifiers` from instance-level artifact fields.

### Modified Capabilities
- `class-resolution-decisions`: a provisional class created because no safe
  existing class matched now always results in a real, catalog-visible
  `kb.ontology_terms` row instead of a headers-only row invisible to
  `kb.ontology_terms_current`.

## Impact

- Code: `server/api/doc-processing/extract-metrics.go`,
  `server/api/doc-processing/ontology_term_property_map.go`,
  `server/api/ontology/keywords/alignment.go`,
  `server/api/ontology/assertions/associate_semantics.go`,
  `server/api/ontology/assertions/metric_lossless_writer.go`,
  `server/api/ontology/assertions/metric_normalizer.go`,
  a new class-synthesizer registry file in `server/api/ontology/assertions/`,
  `server/cmd/config/config.go`.
- Config: `config.toml`/`config.local.toml` — `[ontology_term_property_map]`
  trimmed to class-level fields only; new `[semantic_assertion_property_map]`
  section added.
- DB: no schema migration — `kb.semantic_assertions.qualifiers` and
  `kb.ontology_terms.properties` already exist; this is a behavior-only
  change.
- Tests: existing tests covering `EnsureAcceptedOrCreate`'s call site in
  `extract-metrics.go`, `metricQualifiers`, and
  `resolveOrCreateMetricClass`/`CreateIdentityOnlyClass` need updates; new
  tests cover the `ClassSynthesizer` seam and property-map-driven qualifiers.
- Docs: `KnowledgeStore/doc-repo/bugs/202608/2026082101-bug-auto-promoted-ontology-terms-schema-and-data-loss.md`
  gets a final status update once this change is implemented and archived.
