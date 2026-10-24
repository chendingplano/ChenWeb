## Context

Artifact search (`server/api/kbhandler/search_registry.go`) has a lexical backend, ParadeDB BM25 by
default (`SEARCH_LEXICAL_BACKEND=paradedb`), and when `SEARCH_SEMANTIC_ENABLED` is on a hybrid path
that fuses a lexical top-200 and a vector top-200 with Reciprocal Rank Fusion. The ParadeDB lexical
clause is built by `buildParadeDBSearchWhereClause`:

```sql
(sa.search_document ||| $1 OR sa.primary_label ||| $1 OR … OR sa.source_filename ||| $1)
```

`|||` matches documents that contain any token of `$1`, the raw query. The same builder feeds three
callers: the hybrid query (`queryHybridSearchResultsParadeDB`), the lexical-only search
(`queryRegistrySearchResultsParadeDB`) and the result count (`countRegistrySearchResultsParadeDB`).
Structural filters (artifact type, record, facets) are appended by `appendArtifactFilterClauses` with
positional placeholders starting at a caller-chosen index; the hybrid query uses `$1` (query text),
`$2` (query vector) and `$3…` (filters).

Other handlers already call DeepSeek through a `.models.toml` profile: `loadModelDefByRef` loads the
profile, `defaultNewExtractMetricsClient` builds an OpenAI-compatible JSON client, and setting
`ThinkingType = "disabled"` stops DeepSeek Flash from reasoning, which otherwise costs about 70 s and
12k tokens per call against about 1.2 s and 60 tokens.

## Goals / Non-Goals

**Goals:**
- BM25 matches on the query's keywords instead of on any word of the raw query, in both languages.
- One LLM call per distinct query string; none for repeats while the cache holds them.
- Search behaves exactly as today whenever keyword extraction is off or does not succeed.

**Non-Goals:**
- The PostgreSQL full-text backend (`ts_rank_cd` path). It keeps using the raw query.
- The vector side and the fusion weights (already configurable in `[artifact_search]`).
- Changing default fusion weights; that waits for real query logs.
- A shared or persistent cache across server processes.

## Decisions

**1. Extract once, pass keywords into the builder.** A new `searchQueryKeywords(ctx, query)
[]string` returns the cleaned keyword list, or nil when extraction is off or fails. Each ParadeDB
caller calls it, and `buildParadeDBSearchWhereClause` gains a `keywords []string` parameter: nil
keeps today's clause. The builder stays pure, so the existing SQL tests keep working without an LLM.
Alternative: call the LLM inside the builder. Rejected because it would hide I/O inside a SQL
builder and force every builder test to stub it.

**2. Keyword clause.** For keywords k1…kn:

```sql
((sa.search_document &&& $a OR sa.primary_label &&& $a OR … ) OR (… &&& $b …) OR …)
```

`&&&` requires every token of the keyword (split by the field's jieba tokenizer) to occur in that
field. ORing keywords keeps recall, and ParadeDB's score adds up across matched clauses, so an
artifact matching more keywords ranks higher. This is the "each keyword in full" form measured in
`benchmark/category-index/s9_keywords.py`. Alternative: join the keywords' words with `|||`.
Rejected because it matched all 2,660 metrics; it ranked almost as well (0.84) but gives no narrowing.

**3. Placeholder numbering.** The first keyword takes `$1`, the position the raw query has today,
so the hybrid query's `$2` vector and `$3…` filters keep their numbers. The remaining keywords take
the placeholders after the last filter. The builder returns args in placeholder order; the hybrid
query takes its `$1` from the builder's args instead of the raw query text. Alternative: pass all
keywords as one `text[]` parameter. Rejected because ParadeDB's `&&&` does not accept arrays
(tested: `&&& ARRAY[…]` returns no rows, and `&&& ANY(…)` is an error).

**4. Keyword cleaning.** Keep each keyword's English and Chinese strings, trim them, drop empty
strings, duplicates, strings longer than 64 characters and strings with no letter, digit or CJK
character (they produce no tokens). Keep at most 12 strings. If nothing is left, return nil, which
means fall back to the raw query.

**5. LLM call.** Profile from `SEARCH_QUERY_KEYWORDS_MODEL_NAME` (default
`deepseek-flash-processor`), loaded with `loadModelDefByRef`; `ThinkingType` is always set to
`disabled` regardless of `MODEL_DEFAULT_REASONING_POLICY`, because the call is on the search's
critical path. The client is built once and reused. The prompt comes from
`SEARCH_QUERY_KEYWORDS_PROMPT` (default `prompt-search-query-keywords-v1.md`) under `prompts/`,
loaded once. The call runs under a timeout from `SEARCH_QUERY_KEYWORDS_TIMEOUT_MS` (default 3000).

**6. Cache.** An in-process cache keyed by the trimmed query string holds the cleaned keywords for
24 hours, with at most 2,000 entries (oldest evicted first). Only successful extractions are cached,
so a transient failure is retried on the next search. The count and the result query run one after
the other for one request, so the second always hits the cache. Alternative: cache in the database.
Rejected as not worth the schema for this workload.

**7. Switch.** `SEARCH_QUERY_KEYWORDS` is `on` or `off`; anything else is treated as `off` and
logged once as a warning. The code default is `off`, so environments without the setting behave as
today. The dev environment sets it to `"on"` in `mise.local.toml`, next to the other `SEARCH_*` settings (quoted: a bare TOML boolean in mise `[env]` unsets
the variable).

**8. Logging.** Each extraction is logged with the query length, keyword count, duration and model.
Cache hits are logged at debug level. Fallbacks are logged as warnings with the reason (disabled is
not logged): model or prompt load failure, timeout, LLM error, unparseable or empty result.

**9. Query the partitions, not the parent (found during implementation).** `kb.search_artifacts`
is partitioned by `artifact_type`, and each of its 11 partitions (`kb.search_artifacts_<type>`) has
its own BM25 index. pg_search 0.21.8 rejects BM25 operators on the partitioned parent in every form
(literal or bound type filter, generic or custom plan): "`search_artifacts` does not contain a `USING
bm25` index". So the ParadeDB path has always failed and every search has silently fallen back to
the PostgreSQL full-text path, which made this change's keyword clause unreachable. The ParadeDB
queries (count, lexical-only, hybrid lexical list) therefore read from the partitions: one partition
when the search names one artifact type, otherwise a `UNION ALL` of one branch per partition (all 11,
or those in the `artifact_types` filter), each branch computing `pdb.score` against its own index.
Partition names come only from a fixed list of the 11 artifact types, never from request text; an
unknown type keeps today's parent-table query (and its fallback). The vector CTE keeps reading the
parent, where pgvector works. Alternative: switch the default backend to PostgreSQL full-text.
Rejected because BM25 is the better lexical ranker and the keyword clause needs it. Trade-off: in a
mixed-type search each branch's BM25 scores use that partition's statistics, so scores are not
strictly comparable across types; single-type searches, the common case, are unaffected.

## Risks / Trade-offs

- [About 1.2 s extra latency on the first search of each query string] → cache repeats; the timeout
  bounds the worst case at 3 s before falling back.
- [The LLM drops a term the user needed, e.g. an unusual code] → the prompt keeps codes and symbols
  verbatim, and the vector list is still fused in.
- [Benchmark queries were paraphrases written to avoid the metric's wording, which favours keyword
  extraction over raw matching] → the feature is behind a switch, and fallback is exact current
  behaviour; re-measure with real queries.
- [DeepSeek outage or out-of-balance (402)] → every extraction falls back after the error; search
  keeps working, slower by one failed call per uncached query.
- [Requiring each keyword in full can miss artifacts that use only some words of a multi-word
  keyword] → keywords are OR-ed, both languages are included, and the vector list recovers
  semantic matches.

## Migration Plan

No schema change. Deploy with `SEARCH_QUERY_KEYWORDS` unset (off), then set it to `on` where
wanted. Rollback is setting it to `off`; no restart-time state is involved.
