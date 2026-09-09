## 1. Migration: class-contract search index

- [x] 1.1 Add `project_migrations/20260908000001_create_kb_ontology_class_contract_search.sql` — Up: `CREATE TABLE kb.ontology_class_contract_search` (columns per design D6), `CREATE INDEX ... USING gin (search_vector)`, `CREATE INDEX ... USING hnsw (embedding vector_cosine_ops)`; Down: `DROP TABLE`, keep the `vector` extension. Follow `shared/go/api/goose/goose.md`.
- [x] 1.2 Applied against `miner` (air already picked it up). `\d kb.ontology_class_contract_search` confirms the table + PK + GIN(`search_vector`) + HNSW(`embedding`) + FK to `kb.ontology_term_headers`.
- [x] 1.3 Add a migration presence test in `classcontractsearch` (mirror `class_contract_migration_test.go`) asserting the table and both indexes exist.

## 2. `classcontractsearch` package

- [x] 2.1 Create `server/api/ontology/classcontractsearch/` with a `Store{ DB *sql.DB }` (used `*sql.DB` directly — every caller passes one; no DBX abstraction per ChenWeb "Simplicity First").
- [x] 2.2 `buildDocument(ctx, classTermID)` — composes from `class_term_id`, `module_id`, latest `kb.ontology_terms` `definition`/`scope`, current `kb.ontology_class_contract_revisions.contract_payload` facets (`value_type`, `permitted_unit_term_ids`) + `definition_state`, and up to ~20 distinct instance `metric_name`/`metric_name_en`/`metric_subject` (join `semantic_assertions` → `semantic_decision_candidates` → `kb.metrics`). Unit test `TestContractFacets` / `TestLexemeQuery` cover the pure helpers.
- [x] 2.3 `Reindex(ctx, classTermID, embed)` — builds the document, computes `search_vector` in SQL (`to_tsvector('simple', $5)`), computes `embedding` best-effort (gate on `kbsearch.SemanticSearchEnabled()` + a non-nil `EmbedFunc`; null on miss/dim-mismatch), upserts on `class_term_id` (`ON CONFLICT DO UPDATE`), sets `current_contract_revision_id` / `definition_state` / `module_id` / `instance_count` / `updated_at`. Idempotent.
- [x] 2.4 `MatchSimilar(ctx, classTermID, k, embed)` — loads the class's row (`search_document`, `embedding::text`); parses the stored vector, else embeds the document on the fly; runs the RRF query (lexical CTE `ts_rank_cd(search_vector, to_tsquery('simple', <OR-joined lexemes>))`, semantic CTE `embedding <=> $N::vector`, `FULL OUTER JOIN`, `1/(60+l.rnk)+1/(60+s.rnk)`), excludes `$1`, `LIMIT k`. Returns `[]Match{ClassTermID, Score}`. Lexical-only / semantic-only / neither fallbacks all handled by `buildMatchQuery`.
- [x] 2.5 `BackfillClassContractSearch(ctx, db, embed, limit, reembedAll)` — iterates class terms per `classTermPredicate` (`current_contract_revision_id IS NOT NULL` OR referenced as an assertion `instance_of_term_id`; **not** `term_kind='class'` — synthesized metric classes are `term_kind='metric_definition'`, found when the first `miner` run only picked up the 20 abstract base classes), missing-row first unless `reembedAll`, `Reindex` each, returns `kbsearch.BackfillResult` (`Embedded` = rows reindexed OK).
- [ ] 2.6 Integration test (build tag / `miner`-style DB like `contract_synthesis_integration_test.go`): seed 3 classes + instances, `Reindex` all, assert `MatchSimilar` orders the two "frequency" classes above the unrelated one; assert lexical-only path returns a ranked list with `SEARCH_SEMANTIC_ENABLED` unset. **(needs a test DB)**

## 3. Incremental reindex hook (post-commit, best-effort)

- [x] 3.1 Call site: `ProjectSemanticsProcessor.PostProcessIndex` in `server/api/doc-processing/phase_d.go` — it runs after `associate_semantics` (which resolves classes + commits contract revisions inside its per-metric tx), and it already carries a `Logger`. Reindexes every governed class a metric in the record resolved to (bounded set; `Reindex` is idempotent) rather than an `updated_at` diff.
- [x] 3.2 `refreshClassContractSearchForRecord` in `server/api/doc-processing/class_contract_search_refresh.go` — loads the record's distinct `instance_of_term_id`s, calls `classcontractsearch.Store.Reindex` per class with `embedQueryText`, all under a recover/log guard; returns nothing, logs a per-record summary. Wired at the end of `ProjectSemanticsProcessor.PostProcessIndex` (post-commit).
- [ ] 3.3 Test: a class newly created by a metric write has a search row after the write completes; a forced `Reindex` error is logged and the write result is unchanged. Assert (unit or review-note) that no reindex/embed call happens while the lossless-writer `tx` is open. **(needs a test DB; review-note: `refreshClassContractSearchForRecord` takes `*sql.DB` only and is called after `associate_semantics`/`project_semantics` `.Run` returns, so it is structurally post-commit)**

## 4. Backend: related-metrics endpoint

- [x] 4.1 `server/api/kbhandler/related_metrics_store.go` — `relatedMetricsStore{ DB *sql.DB }` with `metricExists` and `resolveClassTermID(ctx, metricID) (string, error)` (dc→assertion→`instance_of_term_id`, DISTINCT non-empty, first).
- [x] 4.2 `sameClass(ctx, classTermID, excludeMetricID, limit)` — the D4 query (join `semantic_assertions` → `semantic_decision_candidates` `source_artifact_type='metric'` → `kb.metrics` → `kb.inputs`), `GROUP BY` dedupe, deterministic `ORDER BY input_record_id, metric_id`, `LIMIT`.
- [x] 4.3 `similarClass(ctx, classTermID, excludeMetricID, limit, embed)` — calls `MatchSimilar(classTermID, 30, embed)`; lists instances of the matched classes (`a.instance_of_term_id = ANY($1)`, `<> selectedClass`, `<> selectedMetric`, per-class `ROW_NUMBER` cap `<= limit`); attaches each metric's own `class_term_id` + its matched-class score; dedupes by `metric_id` (highest score wins) in Go; sorts by score desc, `input_record_id`, `metric_id`; cuts to `limit`.
- [x] 4.4 `server/api/kbhandler/related_metrics_handler.go` — `GetRelatedMetrics(c echo.Context)` (`EchoFactory`, `CWB_KB_RELM_0xx`): parse+canonicalise `metric_id` → 400; 404 when `kb.metrics` has no such row; parse `scope` (`same_class`|`similar_class`, else 400); `relatedMetricsLimit` (1..200, default 20 / 200 by scope); resolve class → if `""`, 200 with `class_term_id: null`, `results: []`; else dispatch (`similar_class` passes `computeQueryEmbedding`); build the D2 DTO.
- [x] 4.5 `server/api/routes.go` — `apiGroup.GET("/kb/metrics/:metric_id/related-metrics", kbhandler.GetRelatedMetrics)` immediately after the `/graph` line.
- [x] 4.6 Handler test `related_metrics_handler_test.go` (sqlmock via `testhelpers_test.go` harness): malformed id → 400; unknown scope → 400; unknown metric → 404; no-class metric → 200 + null/empty; `same_class` dedupe + self-exclusion; `similar_class` rows carry `matched_class_term_id`+`score`, exclude same-class, order by score.

## 5. Backend: backfill endpoint

- [x] 5.1 `server/api/kbhandler/class_contract_search_backfill_handler.go` — `BackfillClassContractSearch(c echo.Context)` (`CWB_KB_CCSB_0xx`): builds the embedder like `BackfillSearchEmbeddings` (tolerates none → lexical-only), reads `limit` (default 200) + `reembed_all`, calls `classcontractsearch.BackfillClassContractSearch`, returns `{status, model, result}`.
- [x] 5.2 `server/api/routes.go` — `apiGroup.POST("/kb/ontology/class-contracts/backfill-search", kbhandler.BackfillClassContractSearch)` next to `POST /kb/search/backfill-embeddings`.
- [x] 5.3 New direct-DB CLI `server/cmd/class-contract-search-backfill` (the `/api/v1` group needs a Kratos session; a CLI is the headless path — precedent: `server/cmd/metric-contract-backfill`). Needs `docprocessing.EmbedSearchQuery` (exported wrapper over `embedQueryText`). Ran `mise exec -- go run ./server/cmd/class-contract-search-backfill` against `miner`: **78 rows indexed, 77 with embeddings** (56 metric classes + 20 base + 2 capability concepts). Semantic smoke test: nearest neighbour of `振荡频率` (Shaking frequency) is `振幅` (amplitude, cosine 0.84) from the same shaker context — sensible.

## 6. Frontend: model + service

- [x] 6.1 `web/src/lib/components/home3/metric-ontology-explorer/model.ts` — add `related?: 'same_class' | 'similar_class'` to `ChainNode`; add `CHAINS.analysis` with the two nodes (ids `analysis__same_class`, `analysis__similar_class`; labels, `table`, `columns`, `description` per design D9).
- [x] 6.2 `model.test.ts` — assert `CHAINED_SATELLITES` now includes `analysis`, `chainFor('analysis')` returns the two nodes, and `CHAIN_NODE_BY_ID['analysis__similar_class'].parentId === 'analysis'`.
- [x] 6.3 `web/src/lib/services/metricOntologyExplorerService.ts` — `getRelatedMetrics(metricId, scope, limit?)` via the existing `getJson` helper; `RelatedMetricRow` / `RelatedMetrics` types; `PROJECT['analysis__same_class']` and `PROJECT['analysis__similar_class']` mapping the row to the node `columns`.
- [x] 6.4 Service/model test pinning each new `PROJECT` output length to its node's `columns.length`.

## 7. Frontend: content viewer

- [x] 7.1 `content-viewer.svelte` — when `activeNode?.related` is set, take a lazy branch instead of reading `nodeRows`: a `$state` cache keyed `${metricId}::${scope}` holding `{ loading, error, data }`, populated by an `$effect` on `(metricId, activeNode.related)` calling `getRelatedMetrics`.
- [x] 7.2 Render the branch with the existing table shell and states: no metric → "pick a metric"; class unresolved (`class_term_id === null`) → distinct "no resolved governed class yet" note; loading; error; empty; table.
- [x] 7.3 Make rows in this branch activatable — first cell is a `<button class="row-link">` + the row has `role="button"` / keydown that calls `onpickmetric(row.metric_id)`.
- [x] 7.4 Confirm `metric-ontology-explorer-view.svelte` needs no change (`onpickmetric` already deep-links; `counts['analysis']` stays undefined so no badge renders). Verified — no edit.

## 8. Verify end to end (needs `mise dev`)

- [ ] 8.1 With a metric selected, unfold `Analysis`; open `Metrics of Same Class` → table of same-class peers; open `Metrics of Similar Classes` → ≤20 rows with class + score. **(needs `mise dev`)**
- [ ] 8.2 Click a row → URL `metric_id` changes, centre node + record tabs + source pane recenter; re-open the tab for the original metric → served from cache (no new request in the network panel). **(needs `mise dev`)**
- [ ] 8.3 Metric with no resolved class → both tabs show the "no resolved governed class yet" state, no error. **(needs `mise dev`)**
- [ ] 8.4 `GET /api/v1/kb/metrics/:metric_id/graph` response and the other twelve tabs unchanged. **(needs `mise dev`; static review: no edit to `metric_graph_*` or `getMetricGraph`)**

## 9. Docs & spec lineage

- [x] 9.1 `HowTo.md` inspected — it is a dev-environment/neovim how-to with no KB-API or Metric Ontology Explorer section (the sibling `metric-scoped-explorer` change added none either). The two new endpoint contracts are documented in this change's `design.md` (D2/D6/D8) + `specs/*`; recorded in `notes.md`.
- [x] 9.2 Record the spec-lineage note (design "Risks"): when `metric-scoped-explorer` is promoted/archived, the promoted `metric-ontology-explorer` spec must absorb the `Analysis` two-node chain as a non-conflicting addition; capture this in that change's archive checklist too.
- [x] 9.3 Answer the Coding-Best-Practice questions in the PR description: what knowledge changed, which docs updated, which are now stale, what was left undocumented. (Recorded in this change's `notes.md`.)
