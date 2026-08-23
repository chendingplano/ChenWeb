## 1. Migrations

- [x] 1.1 `project_migrations/<ts>_add_metric_subject_concept_id.sql` (goose Up/Down):
  `ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS subject_concept_id TEXT REFERENCES
  kb.keyword_concepts(concept_id)`, mirroring the existing `keyword_concept_id` column. Down:
  `ALTER TABLE kb.metrics DROP COLUMN IF EXISTS subject_concept_id`.
- [x] 1.2 `project_migrations/<ts>_create_kb_metric_value_bucket_map.sql` (goose Up/Down): `dimension`,
  `raw_value` (composite PK), `canonical_bucket TEXT` (nullable, no FK), `status` CHECK'd to
  `proposed|approved|ambiguous` default `proposed`, `occurrence_count` default 0,
  `first_seen_record_id`/`last_seen_record_id`, `note`,
  `create_time`/`create_by`/`modify_time`/`modify_by`. Index on `(dimension, status)`. No seed data.
  Down: `DROP TABLE IF EXISTS kb.metric_value_bucket_map`. Do **not** touch
  `kb.metric_value_range_type_map` in any way.
- [x] 1.3 Confirm via `mise dev` (or a manual goose run) that both migrations apply cleanly against the
  dev DB. Confirmed: the already-running `air` dev server auto-rebuilt on the Go code changes and
  applied both migrations to `miner` (`project_db_migration` shows both `version_id`s
  `is_applied = t`; `kb.metrics.subject_concept_id` and `kb.metric_value_bucket_map` both exist).

## 2. Resolve `subject` in `extract-metrics.go` (strong method)

- [x] 2.1 In `ResolvingMetricsStore.resolveAll` (`server/api/doc-processing/extract-metrics.go`), add a
  second per-metric resolution pass for `metric_subject`, generalizing the existing dedup pattern
  already used for `metric_name`, calling `s.Resolver.ResolveAndObserve(ctx,
  names.ResolveNameRequest{Name: subject, Scope: "metric_subject"},
  names.NameOccurrence{ArtifactType: "metric", ArtifactID: metricID, FieldPath: "metric_subject",
  RawName: subject, Scope: "metric_subject"})` for each distinct, non-empty trimmed `metric_subject`
  value. (Implemented as a separate `resolveMetricSubjects` helper, split out from the metric_name
  pass into its own method — the original single-function draft had a bug: metric_name's own
  early-return on zero entries would have skipped subject resolution too when a batch had no
  metric_name values at all. Caught by task 2.4's tests.)
- [x] 2.2 Write `res.ConceptID` (when non-empty) to `m["subject_concept_id"]`, never overwriting an
  already-populated value.
- [x] 2.3 Thread `subject_concept_id` through the metric persistence path (`SaveMetrics`/
  `UpsertMetrics` SQL) so it round-trips through `kb.metrics`.
- [x] 2.4 Unit tests: a fresh `metric_subject` resolves and writes a concept id; an already-populated
  `subject_concept_id` is never overwritten on re-run; an empty `metric_subject` is skipped.

## 3. `ValueBucketMapper` for `kb.metric_value_bucket_map` (system method, value_type only)

- [x] 3.1 Add `server/api/ontology/assertions/value_bucket_mapper.go`, mirroring `ValueRangeTypeMapper`'s
  exact shape (TTL-cached, invalidate-on-write) keyed by `(dimension, normalized_raw_value)`:
  `Lookup(ctx, dimension, raw string, recordID int64) (canonicalBucket, status string, err error)`.
  `canonicalBucket` non-empty and authoritative only when `status == "approved"`. Reuse
  `NormalizeValueRangeTypeRaw` for the normalization floor.
- [x] 3.2 Unit tests (sqlmock, matching `value_range_type_mapper_test.go`'s style): fresh-miss inserts
  proposed; approved row returns canonical_bucket; proposed/ambiguous never return a bucket; two
  distinct dimensions with the same raw string do not collide; repeated lookup increments
  occurrence_count without a second insert.

## 4. Config schema: `Normalize` becomes a validated string

- [x] 4.1 In `server/cmd/config/config.go`, change `OntologyTermPropertyMapEntry`'s `Identity bool`
  field to keep as-is, remove `Dimension string`, and add `Normalize string`
  (`mapstructure:"normalize"`).
- [x] 4.2 In `LoadConfig`, after unmarshal, validate every `[ontology_term_property_map]` and
  `[semantic_assertion_property_map]` entry's `Normalize` against `{"", "system", "simple",
  "moderate", "strong"}`. Error (fail config load) on any other value, with a distinct, explicit error
  message for `"moderate"` specifically ("recognized method, not yet implemented") versus a genuinely
  unrecognized string ("unrecognized normalize value").
- [x] 4.3 Replace `SemanticAssertionPropertyMapConfig`/`PropertyMap []string` with
  `map[string][]OntologyTermPropertyMapEntry` (same type as `[ontology_term_property_map]`).
- [x] 4.4 Update `GetSemanticAssertionPropertyMap()` to return `map[string][]OntologyTermPropertyMapEntry`.
- [x] 4.5 Unit tests: each valid `normalize` value loads cleanly; `"moderate"` errors with its specific
  message; an arbitrary invalid string errors with the generic message; `""`/unset loads cleanly.
- [x] 4.6 `go build ./server/cmd/config/...` — confirm compiles in isolation.

## 5. Collapse the three builder functions into one, taking a `resolved` map

- [x] 5.1 In `server/api/ontology/assertions/property_map.go`, replace `BuildSignatureProperties`,
  `BuildMappedProperties`, and `ParsePropertyMap` with `BuildConfiguredProperties(entries
  []appconfig.OntologyTermPropertyMapEntry, fieldMap map[string]any, resolved map[string]string)
  map[string]any`: for each entry, `lookupFieldPath`, skip via `isEmptyFieldValue`; if
  `entry.Normalize != ""`, write `{"raw": val, "resolved": resolved[entry.Field]}`; otherwise write the
  plain value. The function never inspects *which* method string is set — only whether it's non-empty.
- [x] 5.2 Delete the now-unused `PropertyMapping` type.
- [x] 5.3 In `metric_normalizer.go`'s `Normalize` loop, construct `bucketMapper :=
  NewValueBucketMapper(db)` alongside the existing `mapper := NewValueRangeTypeMapper(db)`. Build the
  `resolved` map once per row:
  - `metric_name` ← `r.KeywordConceptID.String` (strong, existing)
  - `metric_subject` ← `r.SubjectConceptID.String` (strong, new field on `metricRow`, scanned from
    the new column, tasks 1.1/2.3)
  - `value_class` ← `semid.Normalizer{Version: semid.CurrentNormalizerVersion}.Normalize(r.ValueClass.String).Norm`
    (simple, no DB call)
  - `value_data_type` ← `bucketMapper.Lookup(ctx, "value_type", r.ValueDataType.String, inputRecordID)`
    (system, new)
  - `value_range_type` ← the already-computed `canonicalBucket` from the existing `mapper.Lookup` call
    (system, reused — do not call it twice)
  Propagate `bucketMapper.Lookup` errors the same way `mapper.Lookup`'s error is already handled.
  (Built inside `metricCandidatePayloadForRow` itself, taking `bucketMapper`/`canonicalBucket` as
  parameters, rather than in `Normalize`'s loop directly — keeps the row-shaping logic in one place.)
- [x] 5.4 Update both call sites (class properties, qualifiers) to call `BuildConfiguredProperties`
  with their respective entry list and the same `resolved` map.
- [x] 5.5 Update/replace tests referencing the removed functions (`metric_normalizer_test.go`):
  `value_class` resolves via `semid.Normalizer` (case/whitespace variants normalize identically, no
  DB call observed); `value_type` resolves via the bucket mapper; `range_type` resolves from the
  existing `canonicalBucket`, not a new call; `object_name` never appears wrapped even when
  `identity=true`.

## 6. `matchClassBySignature`/`resolvedSignatureDimensions`: key off `resolved`

- [x] 6.1 In `metric_lossless_writer.go`, change `resolvedSignatureDimensions` to extract
  `entry["resolved"]` (was `entry["term_id"]`); a dimension counts as resolved when non-empty.
- [x] 6.2 Change `matchClassBySignature`'s SQL comparison from `properties->dim->>'term_id' = $value`
  to `properties->dim->>'resolved' = $value`. Ranking/tie-handling logic unchanged.
- [x] 6.3 Unit/integration tests: a unique agreeing candidate on `resolved` values is reused; a
  disagreeing candidate is excluded; `object_name` never contributes to agreement; a `value_type`/
  `range_type` still sitting `proposed` (empty `resolved`) correctly does not count as resolved; zero
  configured/resolved dimensions falls through unchanged. (Existing integration tests already covered
  this shape — updated the `signatureEntry` test helper and ran all 4 live against a real Postgres
  scratch DB; all pass.)

## 7. `withFixedQualifierFields` cleanup

- [x] 7.1 Remove `termProperties()`'s unconditional `ValueType`/`RangeType` lines
  (`server/api/ontology/keywords/alignment.go`) and the now-inapplicable parts of its doc comment.
  (Also fixed two `alignment_test.go` tests whose expected `properties` JSON still included
  `range_type`/`value_type`.)
- [x] 7.2 Remove `withFixedQualifierFields` and its call site in `metric_lossless_writer.go`. Update
  `associate_semantics.go`'s doc comments accordingly.
- [x] 7.3 Remove `metric_lossless_writer_test.go`'s `TestWithFixedQualifierFields` (superseded by task
  5.5's coverage). (Whole file deleted — it contained only that test.)

## 8. Config file rewrite

- [x] 8.1 In `config.local.toml`'s `[[ontology_term_property_map.metric]]`: remove every `dimension =
  "..."` line; set `normalize = "strong"` on `metric_name` and `metric_subject`; set `normalize =
  "simple"` on `value_class`. Leave `ext_info.object_name`'s entry with `identity = true` only, no
  `normalize`.
- [x] 8.2 Add two new `[[ontology_term_property_map.metric]]` entries: `value_data_type` →
  `value_type`, `normalize = "system"`; `value_range_type` → `range_type`, `normalize = "system"`.
- [x] 8.3 Rewrite `[semantic_assertion_property_map]` from its flat `property_map = [...]` list to
  `[[semantic_assertion_property_map.metric]]` table-array entries, one per existing flat entry, no
  `normalize` on any of them. (The file on disk already had `metric_subject`→`subject_name` and
  `ext_info.object_name`→`object_name` entries beyond the 10 flat entries this proposal originally
  discussed — preserved, renaming `subject_name`→`subject` to match the class side's property name.)
- [x] 8.4 Add `[[semantic_assertion_property_map.metric]]` entries: `metric_subject` → `subject`,
  `normalize = "strong"`; `ext_info.object_name` → `object_name`, no `normalize`; `value_data_type` →
  `value_type`, `normalize = "system"`; `value_range_type` → `range_type`, `normalize = "system"`.
- [x] 8.5 Update the explanatory comments above both sections to describe the method-name taxonomy and
  which mechanism resolves which field. Verified by parsing the real `config.toml` +
  `config.local.toml` with viper and running `validateNormalizePropertyMaps()` against the actual
  loaded values (throwaway test, removed after confirming success).

## 9. Verification

- [x] 9.1 `go build ./...`, `go vet ./...`, `go test ./...` from `ChenWeb/` — confirm no new failures
  beyond this repo's existing pre-existing failure catalogue. Both build and vet are fully clean.
  `go test ./...` fails only in `kbhandler`, `llmusage`, `keywords`, `names`, `seed`, `qudt-import` —
  exactly the pre-existing catalogue `fix-metric-property-scope-conflation` already documented
  (sqlmock/query staleness unrelated to this change). Along the way, found and fixed 3 real
  regressions this change caused: `TestMetricsSQLStoreSaveMetricsPersistsMetricCategoriesEn`/
  `TestMetricsSQLStore_UpsertMetrics_OnConflictUpdatesOnlyGivenRows`/
  `TestMetricsSQLStore_GetMetricsByInputRecordID` (`extract-metrics_test.go`) had stale hardcoded
  arg/column counts after `subject_concept_id` was added — fixed. Also fixed two pre-existing
  `alignment_test.go` tests whose expected `properties` JSON still assumed `range_type`/`value_type`
  (see task 7.1).
- [x] 9.2 `TEST_DATABASE_URL` available via `mise.local.toml` (`chenweb_test`, distinct from `miner` —
  per project memory, never point it at `miner`). Ran all 4 signature-matching integration tests plus
  the full `assertions` integration suite (18 tests) against a real Postgres scratch DB — all pass.
- [~] 9.3 **Not run against the live dev DB (`miner`) in this session.** No existing CLI/handler
  triggers a single-record `normalize_assertions`/`associate_semantics` re-run in isolation (would
  need improvised orchestration against a live, actively-used database, and `extract_metrics`
  re-running would trigger real LLM calls) — judged higher-risk than warranted given the mechanism is
  already proven correct via 9.1/9.2 (unit tests + real-Postgres integration tests + real
  `config.local.toml` parsed and validated end to end, task 8.5). Flagged explicitly rather than
  silently skipped; a real reprocess run is a reasonable follow-up once this change is applied to a
  running deployment.
- [x] 9.4 Confirm `kb.metric_value_range_type_map` and its admin handlers are byte-for-byte unchanged.
  Verified via `git diff --stat` — zero diff to `metric_range_type_errors_handler.go` or its migration.
- [x] 9.5 Confirm `kb.governed_property_value_map`/`GovernedPropertyResolver` are untouched. Verified:
  zero diff to `governed_property_resolver.go`/its migration; grepped for any new reference in the
  touched files — none.
- [x] 9.6 Confirm config load rejects `normalize = "moderate"` and an arbitrary invalid string with
  distinct error messages (task 4.5's tests, exercised against the real `LoadConfig` path too).
- [ ] 9.7 Sequence archiving this change after (or together with) archiving
  `governed-class-signature-resolution`. (Not actioned here — archiving is a separate step via
  `/opsx:archive`, out of scope for implementation.)
