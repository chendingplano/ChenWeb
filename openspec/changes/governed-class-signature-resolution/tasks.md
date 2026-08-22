## 1. `kb.governed_property_value_map` migration

- [x] 1.1 Add `project_migrations/<ts>_create_kb_governed_property_value_map.sql` (goose Up/Down),
  DDL exactly per design.md D1/ADR DR3: `dimension`, `raw_value` (composite PK), nullable `term_id
  REFERENCES kb.ontology_term_headers(term_id)`, `status` CHECK'd to
  `proposed|approved|ambiguous|rejected` default `proposed`, `occurrence_count` default 0,
  `first_seen_record_id`/`last_seen_record_id`, `note`, `create_time`/`create_by`/`modify_time`/
  `modify_by`. Index on `(dimension, status)`. No seed data (design.md D5.1 — nothing to preserve).
  Down: `DROP TABLE IF EXISTS kb.governed_property_value_map`.
- [x] 1.2 Confirm via the running `mise dev` dev server (or a manual goose run if it's not up) that
  the migration applies cleanly against the dev DB; check `project_db_migration` for the new row.

## 2. `GovernedPropertyResolver` (generic resolve-or-propose)

- [x] 2.1 Add `server/api/ontology/assertions/governed_property_resolver.go`, mirroring
  `metric_normalizer.go`'s `ValueRangeTypeMapper`/`valueRangeTypeMapCache` shape: a process-wide TTL
  cache keyed by `dimension + "\x00" + normalized_raw_value` (composite key, since this table spans
  multiple dimensions unlike the single-dimension `kb.metric_value_range_type_map`), `fresh`/`reload`/
  `invalidate` methods, same TTL constant value as `valueRangeTypeMapCacheTTL`.
- [x] 2.2 `GovernedPropertyResolver` type with `DB *sql.DB` + cache; `NewGovernedPropertyResolver(db
  *sql.DB) GovernedPropertyResolver` constructing against a shared default cache instance (own
  `defaultGovernedPropertyValueMapCache`, not `ValueRangeTypeMapper`'s — different table).
- [x] 2.3 `Lookup(ctx, dimension, raw string, recordID int64) (termID, status string, err error)`:
  normalize via the existing exported `NormalizeValueRangeTypeRaw` (OD1 floor, per design.md D1 —
  do not write a second normalization function); empty normalized value returns `("", "absent",
  nil)` without touching the DB, matching `ValueRangeTypeMapper.Lookup`'s existing empty-input
  shortcut. On a cache miss, insert `status='proposed'` with `occurrence_count=1` and
  `first_seen_record_id=last_seen_record_id=recordID`, `ON CONFLICT (dimension, raw_value) DO
  NOTHING`, invalidate cache, return `("", "proposed", nil)`. On a `proposed`/`ambiguous` cache hit,
  bump `occurrence_count`/`last_seen_record_id` (no cache invalidation needed for a counter bump,
  matching `touchOccurrence`'s existing behavior). Only `status='approved'` returns a non-empty
  `termID`.
- [x] 2.4 Unit tests (`governed_property_resolver_test.go`, sqlmock — this package's existing test
  style, see `value_range_type_mapper_test.go`): fresh-miss inserts proposed; approved row returns
  term_id; proposed/ambiguous/rejected never return a term_id; two distinct dimensions with the same
  raw string do not collide (composite key correctness); repeated lookup of the same
  `(dimension, raw)` increments occurrence_count without a second insert.

## 3. Config: `[[ontology_term_property_map.<type>]]` table-array (DR4, breaking)

- [x] 3.1 In `server/cmd/config/config.go`, replace `OntologyTermPropertyMapConfig`/its
  `PropertyMap []string` field with `OntologyTermPropertyMapEntry{Field, Property string; Identity
  bool; Dimension string}` (mapstructure tags: `field`, `property`, `identity`, `dimension`) and
  change `AppConfigDef.OntologyTermPropertyMap` to `map[string][]OntologyTermPropertyMapEntry`
  (mapstructure tag stays `"ontology_term_property_map"` — TOML's `[[ontology_term_property_map.
  metric]]` nests directly under that key, no wrapper struct needed per design.md D4).
  `SemanticAssertionPropertyMapConfig`/`GetSemanticAssertionPropertyMap` stay untouched.
- [x] 3.2 Update `GetOntologyTermPropertyMap()` to return `map[string][]OntologyTermPropertyMapEntry`
  (was `[]string`); remove its now-inapplicable doc comment about the flat string format.
- [x] 3.3 In `LoadConfig`, after unmarshal, add the DR4-mandated **warning** (not error): if
  `AppConfig.OntologyTermPropertyMap["metric"]` has zero entries with `Identity == true`, log a
  warning via the existing `logger` param naming the artifact type and missing identity coverage.
- [x] 3.4 `go build ./server/cmd/config/...` — confirm the type change compiles in isolation before
  touching callers (task 4 fixes the one real call site).

## 4. Wire signature resolution into `normalize_assertions` (design.md D2)

- [x] 4.1 In `server/api/ontology/assertions/property_map.go`, add
  `BuildSignatureProperties(ctx context.Context, resolver GovernedPropertyResolver, entries
  []appconfig.OntologyTermPropertyMapEntry, fieldMap map[string]any, recordID int64) (map[string]any,
  error)`: for each entry, resolve `lookupFieldPath(fieldMap, entry.Field)`; skip via the existing
  `isEmptyFieldValue`; if `entry.Identity`, call `resolver.Lookup(ctx, entry.Dimension, rawAsString,
  recordID)` and write `entry.Property -> {"raw": raw, "term_id": termID-or-nil}` (design.md's DR2
  shape — `term_id: null` on `""`/non-approved, not an absent key); otherwise write the plain raw
  value unchanged (today's `BuildMappedProperties` behavior). Returns `nil` for an empty result,
  matching `BuildMappedProperties`'s existing convention. Leave `ParsePropertyMap`/
  `BuildMappedProperties` untouched — still used for `[semantic_assertion_property_map]`.
- [x] 4.2 In `metric_normalizer.go`'s `Normalize`, replace the `classPropertyMappings :=
  ParsePropertyMap(appconfig.GetOntologyTermPropertyMap())["metric"]` line with
  `classPropertyEntries := appconfig.GetOntologyTermPropertyMap()["metric"]` (already grouped by
  artifact type, no parsing needed) and construct `propResolver :=
  NewGovernedPropertyResolver(db)` alongside the existing `mapper := NewValueRangeTypeMapper(db)`.
  `qualifierMappings` (semantic_assertion_property_map) is unchanged.
- [x] 4.3 Thread `ctx`, `propResolver`, `classPropertyEntries`, and `inputRecordID` into
  `metricCandidatePayloadForRow` (or resolve `class_properties` in the `Normalize` loop before
  calling it, whichever keeps the function's existing pure-vs-DB-calling boundary clean — see
  design.md D2's "resolve once, read downstream" rationale) so its `class_properties` build calls
  `BuildSignatureProperties` instead of `BuildMappedProperties(classPropertyMappings, fieldMap)`.
  Propagate any resolver error up through `Normalize`'s existing error return (matching how the
  `mapper.Lookup` error is already handled a few lines above).
- [x] 4.4 Unit tests in `metric_normalizer_test.go`: an identity-configured field with an
  approved/proposed/absent mapping produces the correct `{raw, term_id}` shape in `class_properties`;
  a non-identity field stays a plain value; a field not configured for `metric` never appears.

## 5. Signature-based class resolution (design.md D3)

- [x] 5.1 In `metric_lossless_writer.go`, add a helper (e.g. `matchClassBySignature(ctx, tx,
  input ClassSynthesisInput) (termID string, ambiguousAlternatives []string, err error)`) that: (a)
  extracts every `{raw, term_id}` entry from `input.ExtraProperties` with a non-null `term_id` — the
  occurrence's resolved dimensions; (b) if none, returns `("", nil, nil)` immediately (design.md D3
  gap-fix 1: no vacuous match); (c) otherwise queries `kb.ontology_terms_current` for
  `term_kind = 'metric_definition'` AND `status IN ('included_in_release', 'auto-promoted')`
  (matching `releasedTermExistsSQL`'s existing usability definition) AND, for every resolved
  dimension, `(properties->>dim IS NULL OR properties->dim->>'term_id' IS NULL OR
  properties->dim->>'term_id' = $value)` (the "no contradiction" filter); (d) in Go, count per
  returned row how many of the occurrence's resolved dimensions are non-null-and-equal on that row
  (the "agreement" count); (e) apply design.md D3's ranking: unique max ≥1 → return that termID with
  no alternatives; tie at max ≥1 → return `("", tiedTermIDs, nil)`; max 0 (i.e. no row exceeded the
  vacuous filter with real agreement) → return `("", nil, nil)`.
- [x] 5.2 Wire the helper into `resolveOrCreateMetricClass` as a new first step: unique match →
  return `(termID, semantic.ClassResolvedExisting, nil)` immediately, skipping `SynthesizeClass`
  entirely. No match (including the tie case) → proceed to today's unchanged
  `ConceptID`/header-exists/`SynthesizeClass` logic, but when the tie case produced
  `ambiguousAlternatives`, set the returned `identityState` to `semantic.ClassAmbiguousCandidates`
  instead of `semantic.ClassProvisionalNew` once `SynthesizeClass` creates the new provisional class
  (a tie never resolves as `ClassResolvedExisting` — `created` from `SynthesizeClass` is irrelevant
  to this branch's state, per design.md D3 point 2).
- [x] 5.3 In `writeMetricLossless`, pass the tied alternatives (when present) into
  `ClassResolutionDecisionStore.RecordIfChanged`'s alternatives parameter (currently hardcoded
  `nil`) as `[]classfoundation.ClassResolutionAlternative`, one per tied candidate term_id, `Rank`
  by agreement count (all equal, since they tied), `Evidence` noting "signature tie" — this is the
  first call site to populate that parameter; check `RecordIfChanged`'s existing signature for
  exactly what shape it expects before wiring.
- [x] 5.4 Unit/integration tests (`metric_lossless_writer_test.go` /
  `metric_lossless_writer_integration_test.go`, matching their existing sqlmock/`TEST_DATABASE_URL`
  split): zero resolved dimensions falls through unchanged (regression guard — proves today's
  behavior is preserved); a unique agreeing candidate is reused with `ClassResolvedExisting` and no
  `SynthesizeClass` call; a disagreeing candidate is excluded even when other dimensions would
  otherwise tie it for best; a genuine tie produces a **new** provisional class with
  `ClassAmbiguousCandidates` and populates alternatives.

## 6. Config file rewrite

- [x] 6.1 Rewrite `config.local.toml`'s `[ontology_term_property_map]` section (currently the
  pre-`fix-metric-property-scope-conflation` 12-entry flat list — see proposal.md's Impact section)
  to `[[ontology_term_property_map.metric]]` table-array entries: `identity = true` +
  `dimension = "metric_name"` for `metric_name`; `identity = true` + `dimension = "metric_subject"`
  for `metric_subject`→`subject`; `identity = true` + `dimension = "metric_object_name"` for
  `ext_info.object_name`→`object_name`; `identity = true` + `dimension = "metric_value_class"` for
  `value_class`. Reclassify the remaining 8 fields from the current flat list per design.md D5 point
  2: genuinely class-level-but-non-identity stays here with `identity = false` (no `dimension`);
  genuinely instance-level moves to `[semantic_assertion_property_map]` (flat-string format,
  unchanged) if not already covered there once `fix-metric-property-scope-conflation` lands.
- [x] 6.2 `config.toml` has no `[ontology_term_property_map]` section today (confirmed during
  exploration) — leave it absent unless the operator wants a checked-in default; if added, use the
  same new table-array shape.
- [x] 6.3 Update the explanatory comment above the section (currently describes the flat-string
  finding-6 history) to describe the new identity/dimension shape instead.

## 7. Verification

- [x] 7.1 `go build ./...`, `go vet ./...`, `go test ./...` from `ChenWeb/` — confirm no new failures
  beyond this repo's existing pre-existing failure catalogue (cross-check via `git diff --stat`
  which files changed, same audit style `fix-metric-property-scope-conflation`'s tasks.md 7.2 used).
- [x] 7.2 Confirm `shared/go` was not touched — `go work sync` not needed.
- [x] 7.3 If `TEST_DATABASE_URL` is available in this environment, run the new integration test
  (task 5.4) end-to-end; if not, note it explicitly as unverified-by-inspection-only, matching how
  `fix-metric-property-scope-conflation`'s tasks.md 7.4 already documents the same limitation. Per
  project memory, never point `TEST_DATABASE_URL` at `miner`.
- [x] 7.4 Confirm every `[ontology_term_property_map]` consumer (`metric_normalizer.go`,
  `class_synthesis.go`'s `termProperties()` merge, any admin/debug handler that reads the config)
  still compiles against the new `map[string][]OntologyTermPropertyMapEntry` shape — grep for
  `GetOntologyTermPropertyMap` to confirm no missed call site.
