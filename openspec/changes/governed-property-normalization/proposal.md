## Why

A live-data review of ADR `2026082203`'s implementation (`governed-class-signature-resolution`,
merged same-day) found the shipped mechanism inert. Several earlier drafts of this proposal narrowed
down which mechanism fits which field (`names.Resolver` for open-vocabulary names, a curated bucket
map for `range_type`'s existing table, a new one for the rest). A final review, captured in
`notes.md`, generalized this into a named taxonomy of normalization methods — **system**, **simple**,
**moderate**, **strong** — mapped directly onto spec `2026080403`'s tier ladder, and a data check
against the real corpus corrected one field's assignment (`value_class`'s raw values turned out to be
too clean to need a curated map at all).

Separately, the same review found `kb.semantic_assertions.qualifiers` (the instance) missing
properties its resolved class (`kb.ontology_terms.properties`) carries.

## What Changes

- `OntologyTermPropertyMapEntry.Normalize` becomes a **string enum**, not a bool:
  `""` (unset, no normalization — today's plain-value behavior), `"system"` (field-specific,
  hand-built resolution — e.g. a curated bucket map), `"simple"` (deterministic string cleanup only,
  `semid.Normalizer`'s tier-1 preparation pipeline, no DB), `"moderate"` (tiers 0-3 of
  `names.Resolver`/`KeywordFamily` — table lookups, deterministic scores, no fuzzy matching — **valid
  in config but rejected at load time with a clear error: no code implements it yet, see design.md**),
  `"strong"` (the full tiers 0-5 ladder, already-built, already running in production for
  `metric_name`). Config load errors (not warns) on any value outside this set, including
  `"moderate"` — fail closed on a typo or an aspirational-but-unbuilt setting, matching this ADR
  series' established "report errors instead of silently defaulting" convention.
- **Per-field assignment, settled after a live-data check** (raw-value distribution queried directly
  from `kb.metrics`, not assumed):
  - `metric_name` → **strong**. Reuses `kb.metrics.keyword_concept_id`, already populated by
    `names.Resolver.ResolveAndObserve` today. No new resolution work.
  - `subject` (`metric_subject`) → **strong**. A new `ResolveAndObserve` call in
    `extract-metrics.go`, dedicated scope `"metric_subject"`, new `subject_concept_id` column.
  - `value_class` → **simple**. Corpus check: only 3 distinct values (`requirement`, `reference`,
    `definition`, 56 rows total) — already clean, no synonym clusters. A curated map would be pure
    overhead; `semid.Normalizer` alone is sufficient and this proposal no longer builds one for it.
  - `value_type` (`value_data_type`) → **system**, via a new curated bucket map. Corpus check: 12
    distinct values with real synonym clusters (`numeric`/`integer`/`float`/`number`/`ratio` all mean
    "a number"; `text`/`string`/`qualitative`/`standard_reference`/`table_reference` all mean "not a
    number") — genuinely needs curation, the same shape `range_type` already has.
  - `range_type` → **system**, via its existing, unmodified `kb.metric_value_range_type_map` /
    `ValueRangeTypeMapper` (ADR `2026081401`) — already live, seeded with ~60 approved rows. No new
    code; reuse the `canonicalBucket` already computed once per row.
  - `object_name` (`ext_info.object_name`) → conceptually **system** (a field-specific mechanism is
    the right eventual home, per `notes.md`), but **no mechanism exists yet** — sourced from
    `kb.artifact_objects`/`kb.object_nodes`, a separate identity system this change does not touch.
    Its config entry stays `identity = true` only, `normalize` unset, until a future change actually
    builds that mechanism — setting `normalize = "system"` today with no code behind it would be
    misleading.
  - Nothing is assigned **moderate** in this proposal — no field needs tiers-0-3-only matching today,
    and it requires new code (see below), so building it now would be speculative.
- **`moderate` is deferred, not built**: `KeywordFamily.CandidateNodes` (`keywordfamily.go:108`) is a
  hardcoded, unconditional 0→1→2→3→4→5 sequence with no existing stopping-point parameter — unlike
  `strong` (which is just the existing `ResolveAndObserve`, unmodified), `moderate` would require
  adding a new capability to shared, tested, production code with no current caller. Deferred until a
  real field needs it.
- `BuildConfiguredProperties` (replacing `BuildSignatureProperties`/`BuildMappedProperties`/
  `ParsePropertyMap`) stays mechanism-agnostic: it checks only whether `entry.Normalize != ""`, and
  wraps the field using an already-computed `resolved map[string]string` — it never branches on which
  of the four methods produced that value.
- **BREAKING**: `[semantic_assertion_property_map]` moves to the same
  `[[semantic_assertion_property_map.<type>]]` table-array shape as `[ontology_term_property_map]`.
- `matchClassBySignature`/`resolvedSignatureDimensions` (`metric_lossless_writer.go`) read a
  `resolved` key instead of `term_id`.
- **`kb.governed_property_value_map`/`GovernedPropertyResolver`** (shipped in
  `governed-class-signature-resolution`) remain completely untouched.

## Capabilities

### New Capabilities
- `instance-property-parity`: `kb.semantic_assertions.qualifiers` carries every property its resolved
  class carries, matching the class side's own treatment per field.
- `metric-value-bucket-classification`: the generalized resolve-or-propose bucket map for
  `value_type` (`kb.metric_value_bucket_map`), a plain-string sibling to
  `kb.metric_value_range_type_map`'s existing, unmodified pattern.
- `governed-property-normalization-methods`: the named taxonomy (`system`/`simple`/`moderate`/
  `strong`) as a configuration-level concept, including `moderate`'s deferred status and the
  fail-loudly validation for unrecognized/unimplemented values.

### Modified Capabilities
- `class-identity-signature`: the resolved-signature shape changes from `{"raw":..,"term_id":..}` to
  `{"raw":..,"resolved":..}`; `normalize` becomes a named-method string rather than a boolean.

## Impact

- **New migrations**: `kb.metrics.subject_concept_id TEXT REFERENCES kb.keyword_concepts(concept_id)`;
  `kb.metric_value_bucket_map` (dimension, raw_value → canonical_bucket — today populated only for
  `dimension='value_type'`, structured to hold more dimensions later without a schema change).
- **No changes to `kb.metric_value_range_type_map`, `ValueRangeTypeMapper`, or
  `metric_range_type_errors_handler.go`.**
- **Go code**: `extract-metrics.go` (new `subject` resolution call), `config.go` (`Normalize` becomes
  `string`; validation rejects unrecognized/unimplemented values; `SemanticAssertionPropertyMap`
  restructured), `property_map.go` (three functions collapse into one), a new small resolver
  (`ValueBucketMapper`, mirroring `ValueRangeTypeMapper`'s shape) for the new table,
  `metric_normalizer.go` (`metricRow` gains `SubjectConceptID`; the `Normalize` loop builds the
  `resolved` map: `canonicalBucket` reused for `range_type`, a `semid.Normalizer` call for
  `value_class`, a `bucketMapper.Lookup` call for `value_type`, existing/new concept ids for
  `metric_name`/`subject`), `metric_lossless_writer.go` (keys off `resolved`;
  `withFixedQualifierFields` removed).
- **Breaking config changes**: both property-map sections rewritten; `normalize` values set per the
  table above.
- **Out of scope**: `object_name`'s actual normalization mechanism (future change, `kb.object_nodes`
  scope). `moderate`'s implementation. An admin UI for `kb.metric_value_bucket_map` or
  `kb.governed_property_value_map`. `search_document`/ADR `2026082102` (DR6).
