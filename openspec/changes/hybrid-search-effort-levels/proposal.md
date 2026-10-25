## Why

Hybrid search sends the user's words straight to BM25 and the embedding index. That works when
the input names what it wants, but a request that describes a situation ("after the gas cooler
reaches steady state, how long must I keep recording?") needs someone to work out which metric to
look for and phrase a query for it. The question generator's v3 two- and multi-hop questions
measure exactly this, and single-shot hybrid search ranks some of their targets 11th or 44th.

## What Changes

- Hybrid search gets an **effort level**: `low`, `medium` or `high`.
  - `low`: today's search, the input is the query; no LLM composes queries.
  - `medium` / `high`: an LLM analyses the input and decides whether to search it directly or
    compose new queries, then searches, judges whether the results answer the request, and if not
    composes new queries from another angle and tries again, up to `HYBRID_SEARCH_MEDIUM_EFFORTS`
    (default 2) or `HYBRID_SEARCH_HIGH_EFFORTS` (default 5) attempts.
- Search API routes accept `effort=low|medium|high` (default `low`); the response reports the
  effort, the attempts made and whether a matching result was found.
- The Hybrid Search Benchmark gets an *Effort* run option; with medium or high it adds a fourth
  mode, *Effort*, recording the target's rank after the loop, attempts, found flag, time and the
  queries tried.

## Capabilities

### New Capabilities
- `hybrid-search-effort`: effort levels, the analyse → search → judge → reformulate loop, its
  limits, API parameter and benchmark mode.

### Modified Capabilities
<!-- none in openspec/specs; the benchmark change hybrid-search-benchmark is not archived yet -->

## Impact

- New `server/api/kbhandler/search_effort.go`, prompts `prompt-search-effort-analyze-v1.md` and
  `prompt-search-effort-judge-v1.md`; search handler; benchmark worker, handler, migration, page.
- Each medium/high attempt costs one judge LLM call plus the searches (each with embedding and,
  when on, keyword extraction); the analysis is one more call.
