## 1. Database

- [x] 1.1 Goose migration creating `testbed.hybrid_search_benchmark_runs` and `testbed.hybrid_search_benchmark_results`

## 2. Backend

- [x] 2.1 Run/result types, store (insert, list, load, expire) and score computation
- [x] 2.2 Worker: per-question embedding, keywords, three searches, rank extraction, result insert, heartbeat
- [x] 2.3 Handlers (start, list, get) behind the admin middleware; register routes
- [x] 2.4 Unit tests: rank extraction, scoring, request validation
- [x] 2.5 Live check: start a run against the dev server and confirm ranks are stored

## 3. Frontend

- [x] 3.1 Service `hybridSearchBenchmarkService.ts`
- [x] 3.2 View `hybrid-search-benchmark-view.svelte` (start form, runs, scores, per-question table, polling)
- [x] 3.3 Menu entries (development page only) and `content-panel.svelte` wiring
- [x] 3.4 Messages in `en.json` and `zh-cn.json`; `bun run check` passes

## 4. Docs

- [x] 4.1 Add a benchmark section to spec 2026060201 (hybrid search)

## 5. Search run IDs

- [x] 5.1 Migration: `kb.search_run_id_seq` and `search_run_id` on benchmark results
- [x] 5.2 Allocate a run ID per search (API, agent, benchmark question) and pass run ID + user to the embedding and keyword calls
- [x] 5.3 Tests and live check: both usage events of one search share its run ID; page shows the search run ID per question
- [x] 5.4 Spec 2026060201 §8.2

## 6. Run options, timings, viewer layout

- [x] 6.1 Migration: keyword model/prompt on runs; embedding/keywords/lexical/semantic/hybrid ms on results
- [x] 6.2 Run options (keywords, RRF weights, candidates per list) with configured defaults; per-search keyword override and fresh keyword call
- [x] 6.3 Page: options in the start form, View button, two-group run viewer with prompt and model name, Query time column
- [x] 6.4 Tests and live check; spec 2026060201 §13.2

## 7. Parallelism and keyword failures

- [x] 7.1 Migration: `parallelism` on runs
- [x] 7.2 Worker pool (option 1–32, default 8); results ordered by question
- [x] 7.3 Keyword failure: no retries in the searches, recorded on the result
- [x] 7.4 Tests, live check, spec 2026060201 §13.2
