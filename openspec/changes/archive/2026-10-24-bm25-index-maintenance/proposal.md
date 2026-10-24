## Why

Each `kb.search_artifacts_<type>` partition has its own ParadeDB (pg_search) BM25 index. When a
partition's rows are rewritten (search reindexing deletes and re-inserts a record's rows), pg_search
puts the new documents into a mutable segment, which it searches without the compiled index
structures. On 2026-10-24 the summary partition held all 789 of its documents (about 4,000
characters each) in a mutable segment, so every BM25 predicate took about 0.6 s and an all-types
search with query keywords took 15 s. A rebuild (`REINDEX`) brought one predicate to 1 ms.

Nothing compacts these segments automatically: `VACUUM` leaves them mutable (tested), and
`paradedb.force_merge` is deprecated. So the slowdown returns whenever a partition is rewritten.

## What Changes

- New scheduled job type `maintain_bm25_indexes` ("Maintain Search BM25 Indexes"). Each run inspects
  every BM25 index on the `kb.search_artifacts_<type>` partitions with `paradedb.index_info` and
  rebuilds, with `REINDEX INDEX CONCURRENTLY`, each index whose mutable documents exceed
  `max_mutable_docs` (default 500) or whose deleted documents exceed `max_deleted_ratio` (default
  0.5) of all documents. Searches keep working during a concurrent rebuild.
- Before rebuilding, the job drops invalid leftover indexes from an interrupted concurrent rebuild
  (`…_ccnew…`).
- Parameters: `max_mutable_docs`, `max_deleted_ratio`, `dry_run` (report only) and `limit` (most
  rebuilds per run). Each run records, per index, the document counts, the decision and its time.
- A migration seeds an hourly schedule for the job, as `collect_llm_usage` does.

## Capabilities

### New Capabilities

- `search-index-maintenance`: the scheduled inspection and rebuilding of the search BM25 indexes.

### Modified Capabilities

(none)

## Impact

- Code: new `server/api/kbsearch/bm25_maintenance.go`; job registration in
  `server/api/kbhandler/scheduler_jobs.go`.
- Database: a goose migration that inserts the hourly schedule row into `kb.scheduled_jobs` (no
  schema change).
- Runtime: a rebuild of the largest partition index (entity, about 43,000 rows) is expected to take
  seconds; a run with nothing to rebuild costs one catalog query and one `index_info` call per index.
- The Scheduled Jobs page lists the new job type through the registry; no UI change.
