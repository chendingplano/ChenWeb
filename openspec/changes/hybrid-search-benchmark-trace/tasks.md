## 1. Trace in the hybrid query

- [x] 1.1 `search_registry.go`: split both hybrid queries into a shared CTE builder and the production final SELECT; add `lexical_partitions` (ParadeDB), keep `score` in `lexical`, `distance` in `semantic`, `l.rnk`/`s.rnk` in `fused`
- [x] 1.2 Trace SELECT over the same CTEs: one row per candidate per stage (`lexical_partition`, `lexical`, `semantic`, `fused`) with partition, artifact type/ID, stage rank, score; `lexical` rows also their partition rank (window over `lexical_partitions` in the trace SELECT), `fused` rows their lexical/semantic rank; no `lexical_partition` rows and NULL partition rank on the Postgres backend
- [x] 1.3 `searchTraceSink` (query text, keywords, rows, trace ms, mismatch) on `registrySearchConfig`; `queryWithVectorIndexScan` runs the trace statement in the same `REPEATABLE READ, READ ONLY` transaction only when a sink is set, and compares the fused order with the results
- [x] 1.4 Tests: existing search tests still pass; sqlmock tests for the trace statement on both backends, no trace statement without a sink, mismatch flag
- [x] 1.5 `EXPLAIN` the production ParadeDB hybrid query before and after: same plan

## 2. Storage

- [x] 2.1 Migration: `testbed.hybrid_search_benchmark_queries` and `testbed.hybrid_search_benchmark_candidates` (cascades, CHECKs, primary key per design §4)
- [x] 2.2 Worker: sinks for the hybrid-mode search and the effort loop; build query rows (question first, effort queries matched to attempts by text, untraced attempt queries with an error)
- [x] 2.3 `saveHybridBenchResult`: insert queries and candidates (`unnest`) in the result's transaction
- [x] 2.4 Subtract trace time from `hybrid_ms` and `effort_ms`
- [x] 2.5 Worker tests: low and effort questions produce the expected query rows and stages; timings exclude the trace

## 3. API and page

- [x] 3.1 `GET /api/v1/kb/hybrid-search-benchmark/results/:id/trace?limit=`: top `limit` per stage plus target rows, labels via LEFT JOIN; 400/404; handler test
- [x] 3.2 Service call and `hybrid-search-benchmark-analyze.svelte` dialog: question, target and ranks; queries with source/attempt/keywords; four lists; target highlighted; "no trace stored" state; selectable text
- [x] 3.3 **Analyze** button (last column) in the Questions table opens the dialog; `hsba_*` keys in `en.json` and `zh-cn.json`; `bun run check`

## 4. Verification and docs

- [x] 4.1 Live run (a few questions, low and high effort): the trace's `lexical`/`semantic`/`fused` target ranks equal `lexical_rank`/`semantic_rank`/`hybrid_rank` for every question (SQL check); no mismatch flags; row counts as expected
- [x] 4.2 Devdoc `2026102501-devdoc-hybrid-search-benchmark.md` (trace, tables, the rank check query) and spec `doc-2026060201` §13.2
