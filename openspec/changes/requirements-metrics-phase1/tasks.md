## 1. Provision link column

- [x] 1.1 Add Goose migration `project_migrations/<timestamp>_add_provision_id_to_kb_metrics.sql` (design D1: column, FK `ON DELETE SET NULL`, partial index, column comment citing ADR 2026100603 DR2, Down block), following the `db-migration` skill
- [x] 1.2 Add `provision_id BIGINT` (no FK) to `MetricsSQLStore.ensureMetricsTable` `ADD COLUMN IF NOT EXISTS` list
- [x] 1.3 Expose `provision_id` (nullable) in the `kb.metrics` read queries and JSON responses used by the metric list/detail/wiki handlers, and add `provision_id?: number | null` to the matching types in `web/src/lib/services/kbService.ts`
- [x] 1.4 Integration test against a scratch database: insert with an unknown `provision_id` fails; deleting a referenced provision nulls the link and keeps the metric row
- [x] 1.5 Apply the migration to the dev database and verify the column, constraint and index with `information_schema` / `pg_indexes`

## 2. Retire metric evidence on deletion

- [x] 2.1 Add `EvidenceStore.RetireMetricEvidenceForRecord(ctx, inputRecordID, actor, reason) (int, error)` in `server/api/ontology/assertions/evidence_store.go`, calling `DeleteEvidence` per active `artifact_type='metric'` row of the record
- [x] 2.2 Unit/integration tests: only that record's metric evidence is retired; other records and artifact types stay active; last support moves the assertion to `unsupported` with the reason; second call returns 0
- [x] 2.3 Call the helper before the delete in `MetricsSQLStore.DeleteMetricsByInputRecordID` (reason "metric rows deleted for re-extraction"), logging the retired count with a `CreateDefaultLogger` location id
- [x] 2.4 ~~Call the helper at the benchmark delete sites~~ Not needed (design D2): `store_cleanup.go` deletes the benchmark's own input right after, so its `metric_id`s never recur; `MetricAdapter.Cleanup` has no non-test caller
- [x] 2.5 Test that reproduces the defect: create evidence for `R_mtc_1`, force re-extract record R, and assert that no active evidence references `R_mtc_1` before the lossless writer runs (done as `TestIntegrationReusedMetricIDStartsWithoutEvidence` on a scratch DB, plus a sqlmock test that `DeleteMetricsByInputRecordID` retires before it deletes)
- [x] 2.6 `go build ./...`, `go vet ./...` and the touched packages' tests pass

## 3. Statement-kind classifier (web)

- [x] 3.1 Create `web/src/lib/metric-statement-kind.ts` with `classifyMetricStatement` per design D3, including `reasoning_tags` parsing (array, JSON string, unknown)
- [x] 3.2 Add `web/src/lib/metric-statement-kind.test.ts` covering every scenario in `specs/metric-statement-kind/spec.md`, plus the 69 kinds of the record-416 benchmark rows as a table (15 inspection, 17 delegated, 12 with criterion, 7 value open, 15 test, 1 metric definition, 2 interpretation bounds)
- [x] 3.3 Run the web unit tests and confirm they pass

## 4. Labels in customer-facing views

- [x] 4.1 Add `metric_statement_group_*` and `metric_statement_kind_*` keys to `web/messages/en.json` and `web/messages/zh-cn.json` with real Chinese (要求 / 指标 / 试验参数 / 定义 / 未分类)
- [x] 4.2 Create `StatementKindBadge.svelte` (group label, kind as `title`, text selectable)
- [x] 4.3 Place the badge in `metric-wiki-view.svelte`, `artifact-category-panel.svelte`, `metric-ontology-explorer/metric-search-pane.svelte`, and add a statement-kind attribute in `metric-detail-groups.ts`
- [x] 4.4 `bun run check` passes (no missing keys, no new hard-coded text)
- [ ] 4.5 Check the four views in the running app in English and Chinese with record 416 (requirement rows show 要求/Requirement)

## 5. Docs and wrap-up

- [x] 5.1 Update the ADR 2026100603 change log: Phase 1 implemented (with the change name)
- [x] 5.2 Note in `ChenWeb/KnowledgeStore/Capsules/coding-capsules/doc-processor/extract-metrics-spec.md` that metric deletion retires evidence and that `provision_id` exists but is not yet filled
- [x] 5.3 Commit through jj (ChenWeb only; ADR update in KnowledgeStore as a separate commit)
