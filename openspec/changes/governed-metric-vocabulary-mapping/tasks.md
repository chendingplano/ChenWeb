## 1. Database migrations (goose — use the `db-migration` skill)

- [x] 1.1 `project_migrations/20260814000002_create_kb_metric_value_range_type_map.sql` — `CREATE TABLE kb.metric_value_range_type_map` per design.md D1 (`raw_value` PK, `canonical_bucket`, `status` default `'proposed'` CHECK IN (`proposed`,`approved`,`ambiguous`), `occurrence_count bigint default 0`, `first_seen_record_id`/`last_seen_record_id`, `note`, audit columns matching `kb.ontology_candidates`'s convention). Index on `status` (for triage-by-status queries). Also seeds the six bucket-identity rows (`lower_bound`→`lower_bound`, etc.) that `CanonicalMetricValueRangeType`'s `default` case passed through unchanged — required for exact cutover parity, found while writing design.md.
- [x] 1.2 Same migration: seed `INSERT`s — every synonym in `CanonicalMetricValueRangeType`'s Go switch as `status='approved'` with matching `canonical_bucket` (54 rows total including the 6 identity rows), and every string in `TestResolveMetricValueLeavesAmbiguousVocabularyUnparsed`'s ambiguous list as `status='ambiguous'` (8 rows). Verified live against `miner`: `54 approved / 8 ambiguous`.
- [x] 1.3 `project_migrations/20260814000003_add_assertion_mapping_miss_entry_type.sql` — widened `kb.doc_proc_logs_entry_type_check` to add `'assertion_mapping_miss'`, following `20260814000001_add_resolve_metric_entry_type.sql`'s exact drop/recreate pattern.
- [x] 1.4 `project_migrations/20260814000004_add_kb_metrics_value_range_type_error.sql` — `ALTER TABLE kb.metrics ADD COLUMN value_range_type_error TEXT`, nullable, no backfill.
- [x] 1.5 Applied automatically by the running `mise dev`/air dev server on file save. Verified live against `miner`: `\d kb.metric_value_range_type_map` (11 columns, PK, status index, check constraint), `pg_get_constraintdef` on the widened `doc_proc_logs_entry_type_check` (includes `assertion_mapping_miss`), `\d kb.metrics` shows `value_range_type_error`.

## 2. Governed lookup replaces the hardcoded switch (D1)

- [x] 2.1 Added `ValueRangeTypeMapper` in `server/api/ontology/assertions/metric_normalizer.go`, backed by `valueRangeTypeMapCache` (full-table, 30s TTL, invalidate-on-write). Cache is injectable (`cache *valueRangeTypeMapCache` field) via `NewValueRangeTypeMapper` (production, shared singleton) vs. a test-constructed literal (isolated cache, no cross-test pollution).
- [x] 2.2 `Lookup(ctx, raw string, recordID int64) (canonicalBucket, status string, err error)`. **Contract refined during implementation:** `canonicalBucket` is non-empty *only* when `status == "approved"` — a `'proposed'` row's best-effort guess is persisted to the DB for human review but never returned as usable, so a caller can never accidentally classify against an unapproved guess. Hit/miss/repeat-hit behavior matches DR1 exactly, including the ambiguous-row occurrence-tracking DR1 also calls for.
- [x] 2.3 `CanonicalMetricValueRangeType` retained (not removed) — now documented as a pure reference/test oracle only (DR5 migration source-of-truth, `resolveMetricValueLegacy` test helper); no longer called from any production classification path.
- [x] 2.4 `metricCandidatePayloadForRow` gains a `lookupStatus` parameter written to `payload["value_range_type_lookup"]`; `Normalize`'s loop calls `mapper.Lookup` per row before building the payload.
- [x] **Correctness fix found during implementation:** `resolveMetricValue`'s legacy-fallback condition was rewritten to check `r.ValueRangeType.String == ""` (the raw field) instead of `canonicalBucket == ""` — the two are no longer equivalent post-DR1 (an ungoverned-but-populated `value_range_type` must fall through to honest `unparsed`, never to free-text re-parsing, matching the function's own pre-existing "never a free-text parse of a row that declared structured values" contract).

## 3. `kb.doc_proc_logs` entry type + logger helper (D2)

- [x] 3.1 `EntryTypeAssertionMappingMiss` constant added, included in `allowedDocProcLogEntryType`.
- [x] 3.2 `LogAssertionMappingMiss` added, mirroring `LogResolveMetric`'s shape.

## 4. `associate_semantics` fails on ungoverned vocabulary only (D3)

- [x] 4.1 `AssociateReport.MappingMisses int` added.
- [x] 4.2 `processMetric`'s `!supported` branch checks `p.ValueRangeTypeLookup == "proposed"` and calls a new `deferCandidateAsMappingMiss` (returns outcome `"deferredMappingMiss"`) instead of `deferCandidate` in that case only; every other tag value unchanged.
- [x] 4.3 `Run`'s switch handles `"deferredMappingMiss"` (`Deferred++`, `MappingMisses++`); returns a non-nil aggregate error when `MappingMisses > 0`. **Deviation from the plan, found during implementation:** the `LogAssertionMappingMiss` call itself could not live inside `Run` — `assertions` cannot import `docprocessing` (which already imports `assertions`; a reverse import would cycle). Moved to `phase_d.go`'s `AssociateSemanticsProcessor.PostProcessIndex`, the one call site that already has both the returned report and a `DocProcLogger`. See ADR §8 note.

## 5. Phase C harness fix (D4) — affects every `PostProcessIndexer`

- [x] 5.1 `runPostProcessIndexing`'s error branch now calls `s.persistProcessorRuntimeStatus(ctx, recordID, name, "failed", err.Error())`.
- [x] 5.2 Confirmed `ProjectSemanticsProcessor.PostProcessIndex`'s self-swallowed report-build error is unaffected (still returns nil in that case).

## 6. `extract_metrics` detects at extraction time and self-fails (D6)

- [x] 6.1 `MetricsProcessor.PostProcessIndex` calls a new `checkValueRangeTypeMappings` after existing reindex/object work.
- [x] 6.2 On a miss: `ValueRangeTypeMapper.Lookup` itself upserts the proposal (with guess); `checkValueRangeTypeMappings` then `UPDATE`s that row's `value_range_type_error` and accumulates the raw strings.
- [x] 6.3 If any misses: writes one `LogAssertionMappingMiss` (`doc_proc_name='extract_metrics'`) and returns a non-nil aggregate error; zero misses returns nil as today.
- [x] 6.4 Confirmed: the check runs strictly after all existing extraction/persistence/reindex writes, never gates them.

## 7. Ordering: `normalize_assertions` waits on `extract_metrics` (D6)

- [x] 7.1 `NormalizeAssertionsProcessor.PostProcessDependsOn() []string { return []string{"extract_metrics"} }` added.

## 8. Tests

- [x] 8.1 `value_range_type_mapper_test.go` (7 tests, sqlmock): approved hit, ambiguous hit (+occurrence update), miss+insert (guess never returned), miss with no cue (NULL bucket), repeat-miss updates occurrence not a duplicate insert, empty-raw is "absent" with zero DB calls, cache reuse within TTL (one SELECT for two distinct lookups).
- [x] 8.2 `associate_semantics_test.go`: `TestRunFailsWhenCandidateBlockedOnProposedValueRangeType` and `TestRunSucceedsWhenCandidateDeferredForAmbiguousValueRangeType` — full `Run()`-level sqlmock integration tests (not just `processMetric` in isolation) proving the `"proposed"` vs. other-tag distinction end to end, including the real `MappingMisses`/`Deferred` counts and `Run`'s returned error.
- [x] 8.3 `extract-metrics_test.go`: `TestMetricsProcessorCheckValueRangeTypeMappingsFlagsUnmappedRow` (flags the row, upserts the proposal, logs, returns error) and `...NoMissesReturnsNil` (approved row: no UPDATE, no log, nil error).
- [ ] 8.4 Full integration test (real record through the live Phase D chain via `go test`, not just component-level sqlmock tests) — **not built**. Verified the equivalent behavior piecewise instead: D3's and D6's component tests each confirm their own failed-status/error path, D4's `control_test.go` tests confirm the harness propagation those depend on, and the migrations were verified live against the running dev DB (§1.5). A true end-to-end pipeline run (seed → extract → normalize → associate → assert `has_failed_proc`, approve, drain re-run) is a good candidate for a follow-up if this area gets its own integration-test harness.
- [x] 8.5 `TestResolveMetricValueCanonicalizesProductionVocabularySurvey` and `TestResolveMetricValueLeavesAmbiguousVocabularyUnparsed` rewritten to drive `resolveMetricValue` through `ValueRangeTypeMapper.Lookup` (sqlmock, seeded to mirror the DR5 rows) instead of `CanonicalMetricValueRangeType` directly — both pass unchanged in outcome.
- [x] 8.6 `go build ./...` and `go vet ./...` clean across the workspace. `go test ./...`: every package this change touches passes. 4 pre-existing failing packages confirmed unrelated (none appear in this change's diff): `kbhandler` (fixture/env-dependent, e.g. missing `ARTIFACT_WEB_DIR`), `llmusage` (hardcoded-date temp-path assertion), `server/cmd/qudt-import` (turtle-fixture parsing) — all three already known-broken per the `auto-promoted-governed-terms` change's own task log — plus `server/api/ontology/keywords` (`KEYWORD_RESOLVER_MODE` default-parsing tests, unrelated to metric vocabulary/Phase D, newly observed but outside this diff).

## 9. Documentation (ADR §12)

- [x] 9.1 New file `KnowledgeStore/doc-repo/user-manuals/metric-assertion-semantic-processing-v1.2-en.md` (this KnowledgeStore's established one-file-per-version-bump convention — confirmed via `git log` on the v1.0→v1.1 transition; v1.1 left unmodified). New subsections under §5.1, §5.2, and §9 describing the `'proposed'` failure mode, `kb.metric_value_range_type_map`, `kb.metrics.value_range_type_error`, and the operator approve/correct workflow. Frontmatter bumped to 1.2 with a Change Log entry.
- [x] 9.2 `metric_normalizer.go`'s `CanonicalMetricValueRangeType` doc comment rewritten to state it's retained only as a reference/test oracle, no longer used for production classification.
- [x] 9.3 ADR `2026081401` status updated to `Implemented`; §1 Change Log and §8 both updated with an implementation note, including the DR3 logging-call-site deviation.

## 10. Rollout verification (design.md Migration Plan)

- [ ] 10.1 Spot-check `kb.metric_value_range_type_map` for unexpectedly large `'proposed'` `occurrence_count` values once real traffic has run through the new code (none yet — cutover just landed in this dev environment). Operational follow-up, not a coding task.
- [ ] 10.2 Compare Phase C error-log volume across all `PostProcessIndexer`s before/after rollout once there's a production or staging run to compare against. Operational follow-up, not a coding task.
