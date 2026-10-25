## Why

The Hybrid Search Benchmark stores only where each question's target ended up (lexical, semantic,
hybrid and effort ranks) and the first 10 hybrid IDs. When a rank looks wrong there is no way to
see why: which metrics BM25 returned and with what scores, how the partition lists were merged,
how similar the semantic candidates were, or how RRF combined the two lists. For effort runs, the
queries the loop wrote exist only inside a JSON trace, with no candidates behind them.

## What Changes

- Every traced search of a benchmark question records four candidate lists, each with its rank and
  score:
  1. **BM25 per partition**: the candidates each `kb.search_artifacts_<type>` partition returned,
     with their BM25 score and rank within that partition.
  2. **BM25 merged**: the lexical list after merging the partitions, each candidate with its
     partition, its rank within that partition and its rank in the merged list.
  3. **Semantic**: the vector candidates with their cosine similarity and rank.
  4. **Fused**: the RRF list, each candidate with its RRF score, final rank and its rank in the
     lexical and semantic lists.
- Every query text a question searched is stored once in a new queries table: the question as
  written, and each query the effort loop ran (with its attempt number and its own BM25 keywords).
  Every candidate record references its query.
- The trace comes from the same SQL as the live search (same CTEs, a different final SELECT), run
  in the same read-only snapshot right after the search, so it explains exactly the list the ranks
  were taken from. Timings stored for the run exclude the trace queries.
- An API endpoint returns one question's trace. Each row of the run page's Questions table gets an
  **Analyze** button that opens a dialog showing the question's queries and the four lists, with
  the target highlighted.

## Capabilities

### New Capabilities
- `hybrid-search-benchmark-trace`: per-question candidate traces (BM25 per partition, BM25 merged,
  semantic, fused), the queries table they reference, the trace API and the page's Analyze dialog.

### Modified Capabilities
<!-- none in openspec/specs; hybrid-search-benchmark and hybrid-search-effort-levels are not
     archived yet -->

## Impact

- `server/api/kbhandler/search_registry.go`: the two hybrid queries get shared CTE builders, extra
  CTE columns (BM25 score, distance, input ranks) and an optional trace statement. Live search
  output is unchanged; only callers that set a trace sink run the extra statement.
- `server/api/kbhandler/hybrid_search_benchmark*.go`, `search_effort.go` (no behavior change; the
  sink travels in the search config): benchmark worker, persistence, handler and a new route.
- New migration: `testbed.hybrid_search_benchmark_queries` and
  `testbed.hybrid_search_benchmark_candidates`, cascading from results.
- Storage: about 1,000 candidate rows per traced search at 200 candidates per list (one partition);
  a 200-question low-effort run adds ~200k rows, a high-effort run up to ~16× that.
- DB load: one extra statement per traced search (same cost as the search itself).
- Page `hybrid-search-benchmark-view.svelte` (+ an analyze dialog component), service,
  `web/messages/*.json`.
- Devdoc `2026102501-devdoc-hybrid-search-benchmark.md` and spec `doc-2026060201` §13.2.
