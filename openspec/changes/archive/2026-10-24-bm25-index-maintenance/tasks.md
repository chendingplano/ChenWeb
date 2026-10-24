## 1. Maintenance logic

- [x] 1.1 Add `server/api/kbsearch/bm25_maintenance.go`: `MaintainBM25Indexes(ctx, db, opts, logger)` with catalog discovery, `_ccnew` cleanup, `paradedb.index_info` counts, the rebuild rule, `REINDEX INDEX CONCURRENTLY`, `limit`, `dry_run` and a per-index result
- [x] 1.2 Unit tests with sqlmock: rebuild vs skip decisions, dry run, failure isolation, limit, `_ccnew` cleanup, quoted identifiers

## 2. Scheduler

- [x] 2.1 Register `maintain_bm25_indexes` in `DefaultSchedulerRegistry` with a job function mapping `max_mutable_docs`, `max_deleted_ratio`, `dry_run` and `limit`
- [x] 2.2 Goose migration seeding the hourly schedule (`WHERE NOT EXISTS`), with a down migration

## 3. Verification

- [x] 3.1 Build, vet, kbsearch/kbhandler/scheduler tests (only the two known metric-score failures may remain)
- [x] 3.2 Live: dry run on the dev database lists all 11 indexes with sensible decisions; a real run rebuilds the indexes over threshold, and their BM25 lookups stay fast afterwards; the migration applies and the schedule row exists (2026-10-24: dry run flagged inventory_item, metric, product; real run rebuilt them in ~0.2 s each, a second dry run showed them compact; the migration SQL was verified in a rolled-back transaction and applies at the next server start, because the running air watcher had stopped picking up changes)
- [x] 3.3 Document the job in `KnowledgeStore/Capsules/coding-capsules/llm-wiki/hybrid-search.md`
