## Context

`kb.search_artifacts` is LIST-partitioned by `artifact_type`; each of its 11 partitions has a BM25
index named `kb_search_artifacts_<type>_bm25` (migration `20260603000002/3`). pg_search 0.21.8 keeps
recently written documents in mutable segments (server setting `paradedb.global_mutable_segment_rows
= -1`, the default). Search reindexing (`replaceRegistryRows`) deletes and re-inserts a record's rows,
so a partition rewritten in one go ends up with most or all documents in a mutable segment, and the
old immutable segment keeps them as deleted documents.

Measured on a copy of the summary partition (789 rows): after a delete-and-reinsert a BM25 predicate
took 260 ms; after `VACUUM`, 490 ms and still mutable; after setting the index option
`mutable_segment_rows = 0`, 230 ms; after `REINDEX INDEX CONCURRENTLY`, 1 ms. On the live summary
index a plain `REINDEX` took 0.3 s and turned 675 ms into 1 ms. `paradedb.force_merge` reports that it
is deprecated in favour of `VACUUM`.

The scheduler (`server/api/scheduler`) runs registered job types (`DefaultSchedulerRegistry`) on
schedules stored in `kb.scheduled_jobs`, recording each run's result in `kb.scheduled_job_runs`.

## Goals / Non-Goals

**Goals:**
- Keep every search BM25 index compact without operator action.
- Never block searches while doing so.
- Make each decision visible in the run result and the logs.

**Non-Goals:**
- Changing pg_search settings or index options (tested; does not solve it).
- Other indexes (HNSW vector indexes, the testbed tables).
- Probing query latency; the decision uses segment counts only.

## Decisions

**1. Rebuild with `REINDEX INDEX CONCURRENTLY`.** It is the only tested operation that restores
1 ms lookups, and it builds the new index beside the old one, so searches keep running. It cannot
run inside a transaction block, so it is issued with `ExecContext` on the pool (autocommit).
Alternatives: plain `REINDEX` (blocks reads of the index while it runs), `VACUUM` (no effect),
`mutable_segment_rows = 0` (still slow after a rewrite).

**2. Decide from `paradedb.index_info(index, false)`.** For each index, sum `num_docs` over mutable
segments (mutable documents), `num_docs` over all segments (live documents) and `num_deleted`
(deleted documents). Rebuild when mutable documents > `max_mutable_docs` (default 500) or deleted /
(live + deleted) > `max_deleted_ratio` (default 0.5). The 500 default is above the 300–400 short
mutable documents seen on partitions whose queries stay at about 90 ms, and below the 789 that made
summary slow.

**3. Discover indexes from the catalog.** Select BM25 indexes (`indexdef ILIKE '%USING bm25%'`) on
tables `kb.search_artifacts_%`, so a new artifact-type partition is covered without code changes.
Index names go into `REINDEX` as quoted identifiers (`pq.QuoteIdentifier`); they come from the
catalog, never from job parameters.

**4. Clean up interrupted rebuilds first.** A failed `REINDEX CONCURRENTLY` leaves an invalid index
named `<index>_ccnew` (or `_ccnew<n>`). The job drops invalid indexes on the partitions whose name
contains `_ccnew` with `DROP INDEX CONCURRENTLY` before inspecting, and records them.

**5. Bounded and failure-isolated.** `limit` (default 11) caps rebuilds per run; a failed index is
recorded and the job moves on; the job fails only when it cannot list the indexes. `dry_run` reports
what would be rebuilt without rebuilding.

**6. Placement.** The SQL logic lives in `kbsearch` (`MaintainBM25Indexes`), next to the search
registry it serves; `kbhandler/scheduler_jobs.go` only maps job params to options, as the other jobs
do.

**7. Schedule.** A migration inserts "Maintain search BM25 indexes (hourly)", interval 3600 s, with the
default parameters, guarded by `WHERE NOT EXISTS` on the job type; the down migration deletes it.
Hourly keeps the worst case (a partition rewritten just after a run) to under an hour.

## Risks / Trade-offs

- [A concurrent rebuild needs disk for a second copy of the index and extra CPU] → indexes are small
  (largest 16 MB); `limit` caps rebuilds per run.
- [The thresholds are judged from one partition's measurements] → they are job parameters and can be
  changed on the schedule row without a deploy.
- [Rebuilding during heavy reindexing could rebuild an index that is rewritten again minutes later]
  → harmless; the next hourly run compacts it again.

## Migration Plan

Deploy the code, then the migration seeds the schedule. Rollback: disable or delete the schedule row
(or run the down migration); no data or schema to restore.
