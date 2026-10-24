## 1. Prompt and configuration

- [x] 1.1 Add `prompts/prompt-search-query-keywords-v1.md` (from the benchmark's `prompt-catidx-query-keywords-v1.md`), with input of one query and output `{"keywords": [{"en", "zh"}]}`
- [x] 1.2 Add `SEARCH_QUERY_KEYWORDS="on"`, `SEARCH_QUERY_KEYWORDS_MODEL_NAME`, `SEARCH_QUERY_KEYWORDS_PROMPT` and `SEARCH_QUERY_KEYWORDS_TIMEOUT_MS` to the ChenWeb dev env next to the other `SEARCH_*` settings (`mise.local.toml`, untracked; quoted values), and check them with `mise env`

## 2. Keyword extraction

- [x] 2.1 Add `server/api/kbhandler/search_query_keywords.go`: switch parsing (on/off, warning on other values), lazy model and prompt loading with reasoning disabled, and the LLM call under the configured timeout
- [x] 2.2 Implement keyword cleaning (trim, dedupe, drop empty, longer than 64 characters or without letter/digit/CJK, keep at most 12)
- [x] 2.3 Implement the in-memory cache (24 h, 2,000 entries, oldest evicted, successes only)
- [x] 2.4 Log extractions, cache hits (debug) and fallbacks with reasons, using a logger from `CreateDefaultLogger`
- [x] 2.5 Unit tests with a stubbed extractor: cleaning, cache hit, failure not cached, timeout fallback, switch off makes no call

## 3. BM25 clause

- [x] 3.1 Add a `keywords []string` parameter to `buildParadeDBSearchWhereClause`: nil keeps the raw-query `|||` clause; otherwise per-keyword `&&&` clauses OR-ed, first keyword at `$1`, others after the filter placeholders
- [x] 3.2 Use `searchQueryKeywords` in `countRegistrySearchResultsParadeDB`, `queryRegistrySearchResultsParadeDB` and `queryHybridSearchResultsParadeDB`; the hybrid query takes `$1` from the builder args
- [x] 3.3 Builder and sqlmock tests: raw clause unchanged without keywords; keyword clause and placeholder order with and without filters; hybrid keeps the vector at `$2`
- [x] 3.4 Read the BM25 queries (count, lexical-only, hybrid lexical CTE) from the `kb.search_artifacts_<type>` partitions: one partition for a single type, `UNION ALL` branches otherwise; table names only from the fixed type list; unknown type keeps the parent table
- [x] 3.5 Tests for 3.4: single-type, all-types and `artifact_types` filter SQL; unknown type; plus a live check that ParadeDB search no longer errors

## 4. Verification

- [x] 4.1 `go build ./...`, `go vet` and the kbhandler and config test packages (only the two known metric-score failures may remain)
- [x] 4.2 Live check against the dev database: a search with keywords on returns results and logs one extraction; the same search again hits the cache; with a bad model name the search falls back and still returns results
- [x] 4.3 Re-run `benchmark/category-index/s9_keywords.py` with the production prompt and confirm BM25 R@10 is about the benchmark's 0.86 (result: 0.80 with one query per call, as production calls it; the batched benchmark prompt gets 0.86 and the same prompt one query per call 0.83, with run-to-run noise of about ±0.03. Hybrid at weight 1.0: 0.90 vs 0.85 for the raw query)
- [x] 4.4 Update the hybrid-search API doc (`KnowledgeStore/doc-repo/misc/202606/2026061104-hybrid-search-api.md`) with the keyword step and its settings
