## Context

A benchmark question is searched by `benchmarkHybridSearchQuestion` three times through the
production hybrid query (`queryHybridSearchResultsParadeDB`, or `queryHybridSearchResults` for the
Postgres backend) with RRF weights 1/0, 0/1 and the run's weights. Medium/high runs also call
`searchWithEffort`, which searches every query it writes through `queryRegistrySearchResults`
(same hybrid query, top 20 per query) and keeps attempts in `effort_trace` JSON.

The ParadeDB hybrid query is one statement with three CTEs:

- `lexical`: a `UNION ALL` of one BM25 branch per partition
  (`kb.search_artifacts_<type>`, `ORDER BY pdb.score DESC LIMIT n` each), re-ranked across
  partitions by raw score and cut to n;
- `semantic`: nearest n by cosine distance on the parent `kb.search_artifacts`;
- `fused`: full outer join, `w_l/(60+l.rnk) + w_s/(60+s.rnk)`.

The final SELECT keeps only the fused rows (ordered by score, artifact ID). The per-partition
scores, the merged lexical scores and the distances are computed and thrown away.
`kb.search_artifacts` is `LIST (artifact_type)` partitioned, so a row's partition is
`kb.search_artifacts_<artifact_type>`. The benchmark searches only `metric`, so today the merged
lexical list equals the single partition's list; the trace still records both so other purposes
can use it.

## Goals / Non-Goals

**Goals:**
- For each traced search, store the four candidate lists the hybrid query built (BM25 per
  partition, BM25 merged, semantic, fused) with scores and ranks.
- Store each query text a question searched (the question; every effort query) once, and make every
  candidate row reference it.
- The trace must describe exactly the list the stored ranks come from, and must not change live
  search output or the stored timings.

**Non-Goals:**
- No trace for live search API calls; only benchmark runs set a trace sink.
- No trace of the lexical-only (1/0) and semantic-only (0/1) mode searches: weights only change
  the fusion, so their lexical and semantic lists are the ones the hybrid-mode trace already holds.
- No stored list for the effort loop's cross-query fusion. It is the RRF of each query's first 20
  fused candidates, all of which are stored.
- No per-run switch to turn tracing off (see Risks).

## Decisions

### 1. Same CTEs, second statement, same snapshot

The hybrid query builders are split into a shared CTE builder and two final SELECTs:

- the production SELECT, unchanged in output;
- a trace SELECT that returns one row per candidate per stage from the same CTEs.

The CTEs gain the values the trace needs: a `lexical_partitions` CTE holding the per-partition
branches (`artifact_type, artifact_id, score`); `score` kept in `lexical`; `distance` kept in
`semantic`; `l.rnk`/`s.rnk` kept in `fused`. A CTE referenced once is inlined by PostgreSQL, so the
production plan stays the same (checked with `EXPLAIN` in the tasks).

When the search config carries a trace sink, `queryWithVectorIndexScan` opens the transaction as
`REPEATABLE READ, READ ONLY`, runs the production statement, then the trace statement (same
arguments minus page size and offset), and appends the trace to the sink. Without a sink nothing
changes.

*Alternatives.*
- A separate trace query written for the benchmark: rejected, it would drift from live search,
  which the benchmark exists to measure ("no copy of the search").
- One statement returning production rows plus stage rows: exact by construction, but the
  production path would then run a different final SELECT in trace mode, and every caller's scan
  would change. Two statements in one snapshot give the same list with the production statement
  untouched.
- Replacing the three mode searches with one traced search (lexical rank = trace lexical rank):
  would also be exact, but it changes how lexical/semantic times are measured. Out of scope.

### 2. Stages and scores

| stage | rows | `score` | `stage_rank` |
|---|---|---|---|
| `lexical_partition` | each partition branch, ≤ n per partition | BM25 (`pdb.score`) | within the partition, by score then artifact ID |
| `lexical` | merged list, ≤ n | BM25 | merged rank (the `rnk` RRF used); `partition_rank` = rank within its partition |
| `semantic` | ≤ n | cosine similarity = 1 − distance | `rnk` |
| `fused` | ≤ 2n | RRF score | final order (score, artifact ID), the order ranks are taken from |

`lexical` rows carry two ranks: `stage_rank` (merged) and `partition_rank` (the same candidate's
rank in its partition's list, as in its `lexical_partition` row). The trace SELECT computes
`partition_rank` with a window over `lexical_partitions` joined to `lexical`, so the production
CTEs get no extra window function. `fused` rows also carry `lexical_rank` and `semantic_rank`
(NULL when the candidate was not in that list), so each RRF score can be checked by hand. Every
row carries `partition_name` and `artifact_type`.

**Postgres backend.** Its lexical CTE is one `ts_rank_cd` query over the parent table, not per
partition, so it records no `lexical_partition` rows and `partition_rank` is NULL; `lexical`
scores are `ts_rank_cd`.

BM25 scores of different partitions come from different indexes, so their raw scores are not
strictly comparable; the merged stage shows the order the live query produces anyway. Making that
visible is part of the point.

### 3. Trace sink in the search config

`registrySearchConfig` gets `trace *searchTraceSink` next to `call`. A pointer survives the copies
the benchmark and the effort loop make, so `searchWithEffort` needs no change: every
`searchEffortQuery` call of a traced question appends its own entry. An entry holds the query
text, the BM25 keywords that query used, the candidate rows, the trace statement's duration and
whether the trace matched the results (decision 6). Each question gets its own sinks, so parallel
workers never share one.

The worker uses two sinks per question: one for the hybrid-mode search, one for the effort loop.

### 4. Tables

`testbed.hybrid_search_benchmark_queries`, one row per query a question searched:

| column | |
|---|---|
| `id` BIGSERIAL PK | |
| `result_id` → `hybrid_search_benchmark_results(id)` ON DELETE CASCADE | |
| `seq` INTEGER | order within the question, 1 = the question |
| `source` TEXT | `question` or `effort` |
| `attempt` INTEGER NULL | effort attempt (1-based); NULL for `question` and for the fallback search after a failed analysis |
| `query_text` TEXT | as searched |
| `keywords` TEXT[] | the BM25 keywords of this query (empty when off or failed) |
| `error_msg` TEXT | e.g. no trace because the search fell back to lexical-only |

`testbed.hybrid_search_benchmark_candidates`, one row per candidate per stage:
`query_id` (→ queries, ON DELETE CASCADE), `stage` (CHECK in the four values), `partition_name`,
`stage_rank` (> 0), `partition_rank` (`lexical` rows only), `artifact_type`, `artifact_id`,
`score` DOUBLE PRECISION, `lexical_rank`, `semantic_rank` (`fused` rows only); primary key `(query_id, stage, partition_name, stage_rank)`. Column names avoid
the SQL keywords `partition` and `rank`. The question as written is a query row too, so every
candidate has a non-null `query_id`.

Effort queries are matched to attempts by text: `cleanSearchEffortQueries` never repeats a query
within a question, so each text is in at most one attempt. An attempt query with no sink entry
(lexical-only fallback, failed search) still gets a query row, with an `error_msg` and no
candidates.

### 5. Full lists, written with the result

All candidates are stored, not a top-k: the user asked for the retrieved candidates, and a missed
target is explained by which lists it was absent from. Queries and candidates are inserted in the
transaction that inserts the result row (`saveHybridBenchResult`), candidates as one
`INSERT … SELECT FROM unnest(...)` per query, so a result never exists without its trace.

### 6. Consistency check

After the trace statement, the sink compares the production result IDs (in order) with the first
as many `fused` IDs. A difference marks the query's `error_msg` "trace differs from the search
results". Inside one snapshot only HNSW traversal over concurrently inserted, invisible rows could
cause it.

### 7. Timings exclude the trace

The worker subtracts each sink's recorded trace time: `hybrid_ms` is the search minus its trace
statement, `effort_ms` the loop minus all its trace statements. Stored timings stay comparable
with earlier runs.

### 8. API and page

`GET /api/v1/kb/hybrid-search-benchmark/results/:id/trace?limit=20` (admin-only like the other
routes) returns the result's target and its queries in `seq` order, each with the four stages: the
first `limit` rows (default 20, 1–1000) plus the target's row when it ranks lower, and labels
joined from `kb.search_artifacts` (LEFT JOIN: a re-indexed or deleted artifact shows its ID only).

*Superseded by openspec `hybrid-search-benchmark-analyze-page`: Analyze now opens a new tab with the log, a PDF panel and metric details; the dialog below was removed.* The Questions table gets an **Analyze** button per question (last column). It opens a dialog
headed by the question, its target and its four ranks, listing the queries (the question, then
effort queries by attempt, with keywords) and for each query the four lists, target row
highlighted. Results from earlier runs say that no trace was stored. Text is selectable; all text
is in `web/messages/*.json` (`hsba_*`, component `hybrid-search-benchmark-analyze.svelte`).

## Risks / Trade-offs

- [Storage: ~1,000 rows per traced search at n = 200, up to ~16 searches per high-effort question]
  → a 200-question low run adds ~200k rows; deleting a run cascades to its trace. If it becomes a
  problem, a per-run trace option or top-k + target rows can be added later.
- [Extra DB load: one more statement per traced search, same cost as the search] → timings exclude
  it, but with parallel queries it adds to the load other questions see (already a documented
  limitation).
- [Production SQL text changes (new CTE, extra columns)] → output is unchanged; existing search
  tests and an `EXPLAIN` comparison confirm it.
- [Trace and results disagree] → flagged per query (decision 6), never silently.

## Migration Plan

One goose migration creating both tables (Down drops them). Earlier runs keep working and show no
trace. Rollback: revert the code; the tables are only read by the trace endpoint.

## Open Questions

- Should live search API calls be traceable too (e.g. a debug flag)? Not needed for the benchmark;
  the sink makes it possible later.
