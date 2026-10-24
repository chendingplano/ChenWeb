## Why

The BM25 side of artifact search matches any word of the raw query (`field ||| query`). Filler
words such as "how", "much" and "allowed" match nearly every document, so for a typical query BM25
matches the whole metric corpus and ranks it poorly. In the category-index benchmark (212 queries,
2,660 metrics, `benchmark/category-index`) raw-query BM25 found the target in its top 10 for 47% of
queries, and for 20% of English queries. Fused with equal weight, it made hybrid search worse than
vector search alone (0.87 vs 0.95).

Extracting keywords from the query with an LLM and requiring each keyword to match in full raised
BM25 to 0.86 (0.82 for English), narrowed the matched set from all 2,660 metrics to a median of 162,
and let hybrid search reach 0.96 at a lexical weight of 0.5 (`s9_keywords.py`).

## What Changes

- Before a search runs, an LLM turns the query into 2 to 6 keywords, each in English and Chinese,
  dropping question words and filler and keeping technical terms, codes and symbols exactly.
- With the ParadeDB backend, the BM25 clause requires every word of a keyword to match in one field
  (`&&&`) and ORs the keywords, so artifacts matching more keywords score higher. This applies to the
  hybrid query's lexical list, the lexical-only search and the result count.
- If keyword extraction is disabled, fails, times out or returns nothing, search uses the raw query
  exactly as today. A search never fails because of keyword extraction.
- Keywords are cached in memory per query string, so the count and the result query share one LLM
  call and repeated searches make none.
- Configuration by environment variables: `SEARCH_QUERY_KEYWORDS` (`on`/`off`, default `off`),
  `SEARCH_QUERY_KEYWORDS_MODEL_NAME` (a `.models.toml` profile, default
  `deepseek-flash-processor`), `SEARCH_QUERY_KEYWORDS_PROMPT` (default
  `prompt-search-query-keywords-v1.md`) and `SEARCH_QUERY_KEYWORDS_TIMEOUT_MS` (default 3000).
  Model reasoning is always disabled for this call, which is on the search's critical path.
- The dev environment turns the feature on (`mise.local.toml`, which is untracked, next to the other `SEARCH_*` settings).

## Capabilities

### New Capabilities

- `search-query-keywords`: LLM keyword extraction for the lexical side of artifact search, the
  keyword BM25 clause, fallback to the raw query, caching and configuration.

### Modified Capabilities

(none)

## Impact

- Code: `server/api/kbhandler/search_registry.go` (ParadeDB WHERE builder, lexical, count and hybrid
  paths) and a new keyword-extraction file in `server/api/kbhandler`.
- Prompt: new `prompts/prompt-search-query-keywords-v1.md`.
- Config: dev env vars in `mise.local.toml` (untracked). `config.toml` is unchanged; fusion weights stay in
  `[artifact_search]`.
- Runtime: one LLM call per uncached query, about 1.2 s and about 60 completion tokens with DeepSeek
  Flash and reasoning off. It adds latency to the first search for each query string.
- Unchanged: the PostgreSQL full-text backend (`SEARCH_LEXICAL_BACKEND=postgres`), the vector side,
  and search APIs and responses.
