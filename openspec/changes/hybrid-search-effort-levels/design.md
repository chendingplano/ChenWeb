## Context

`queryRegistrySearchResults` runs one hybrid search for one query text. Query keywords
(`searchQueryKeywords`) already use an LLM per query; search run IDs (`searchCall`) tag every LLM
call of a search.

## Decisions

**Loop.** `searchWithEffort`:
1. *Analyse* (one LLM call): the request → `strategy` (`direct` | `compose`), `need` (what would
   answer it), `queries` (1–3). `direct` means the first attempt searches the request as written.
2. *Attempt* (up to the effort's maximum): run every current query through the existing hybrid
   search (top 20 each), fuse all lists seen so far with RRF (k = 60), and send the request, the
   need, the queries tried and the top 10 fused candidates (label, secondary label, snippet) to a
   *judge* LLM call → `found`, `matching_ids`, `next_queries`, `reason`.
3. Stop when `found`, when the maximum is reached, or when the judge proposes no new query. The
   result is the matching IDs first (judge order), then the fused list.

**Found means judged, not known.** Live search has no ground truth, so the judge decides whether a
candidate answers the request. The benchmark never shows the loop the target; it scores the final
list against it.

**Attempts.** One attempt = one round of searches plus one judgement. `medium` = 2, `high` = 5 by
default (`HYBRID_SEARCH_MEDIUM_EFFORTS`, `HYBRID_SEARCH_HIGH_EFFORTS`; values below 1 fall back to
the default).

**Model.** `HYBRID_SEARCH_EFFORT_MODEL_NAME` (default `deepseek-flash-processor`), reasoning
disabled, `HYBRID_SEARCH_EFFORT_TIMEOUT_MS` per call (default 20000). Prompts are files in
`prompts/`. Calls carry the search run ID and call reasons `search_effort_analyze` /
`search_effort_judge`.

**Failures degrade, never fail.** Analysis failure → low-effort search of the request. Judge
failure → stop and return the fused list so far. The trace records the error.

**API.** `effort` query parameter on the registry search routes; unknown values → HTTP 400. For
medium/high, `total` is the length of the final list and pages are cut from it. The response adds
`effort` and `effort_trace` (attempts with queries, found, matching IDs, reason).

**Benchmark.** Run option `effort` (default `low`). For medium/high each question also runs the
loop; results store `effort_rank`, `effort_found`, `effort_attempts`, `effort_ms`,
`effort_trace`; scores gain mode `effort`. Lexical/semantic/hybrid stay single-shot for
comparison.

## Risks / Trade-offs

- [Latency and cost: high effort can make ~6 LLM calls plus up to 15 searches] → opt-in;
  `low` stays the default.
- [The judge can accept a wrong metric] → the benchmark measures how often the final list holds the
  real target.
