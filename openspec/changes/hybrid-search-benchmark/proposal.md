## Why

The Question Generator now fills `testbed.generated_questions` with known-item questions, each
written to find one metric (`purpose = 'search_metrics'`). Nothing uses them yet: the only search
measurements are the offline Python scripts in `benchmark/category-index/`, which query testbed
copies of the search rows rather than the live search code. A page that runs the stored questions
through the real metric search lets us measure every search change (keywords, weights, embedding
model, index rebuilds) the same way and keep the results.

## What Changes

- New development-page menu entry **System Admin → Benchmark → Hybrid Search Benchmark → Run
  Benchmark**.
- The page starts a benchmark run for a question purpose (only `search_metrics` today), lists past
  runs, and shows a selected run's scores and per-question ranks, refreshing while a run is in
  progress.
- A background worker runs each question through the live metric search (`kb.search_artifacts`,
  artifact type `metric`, whole corpus) three times — lexical only, semantic only, and hybrid with
  the configured RRF weights — and records the rank of the target metric in each list (top 200,
  else a miss).
- Scores per mode: R@1, R@5, R@10 and MRR, for all questions and broken down by hop type and
  language.
- Two new tables in `testbed`: benchmark runs (with the search settings in force) and per-question
  results.
- New admin-only API under `/kb/hybrid-search-benchmark`.

## Capabilities

### New Capabilities
- `hybrid-search-benchmark`: running stored generated questions through the live search, storing
  runs and per-question ranks in `testbed`, and the admin page that starts and reviews runs.

### Modified Capabilities
<!-- none -->

## Impact

- Backend: new files in `server/api/kbhandler/` (handler, worker, store); routes in
  `server/api/routes.go`. Reuses the existing hybrid query functions in `search_registry.go`
  unchanged.
- Database: goose migration creating `testbed.hybrid_search_benchmark_runs` and
  `testbed.hybrid_search_benchmark_results`.
- Frontend: new view under `web/src/lib/components/home3/`, a service in `web/src/lib/services/`,
  menu entries in `nav-rail.svelte`, wiring in `content-panel.svelte`, messages in both
  `web/messages/*.json`.
- Each question costs one query embedding and, when `SEARCH_QUERY_KEYWORDS` is on, one keyword
  LLM call (cached per query string).
