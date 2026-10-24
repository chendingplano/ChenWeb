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
