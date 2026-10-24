-- +goose Up
-- openspec bm25-index-maintenance: hourly schedule for the maintain_bm25_indexes job, which rebuilds
-- (REINDEX INDEX CONCURRENTLY) kb.search_artifacts_<type> BM25 indexes left with many documents in
-- pg_search mutable segments or many deleted documents. Thresholds are the job defaults and can be
-- changed on the schedule row. Added only when no schedule for the job type exists.
INSERT INTO kb.scheduled_jobs (name, job_type, interval_seconds, params, enabled, run_once, next_run_at)
SELECT 'Maintain search BM25 indexes (hourly)', 'maintain_bm25_indexes', 3600,
       '{"max_mutable_docs": 500, "max_deleted_ratio": 0.5}'::jsonb, true, false, NOW()
WHERE NOT EXISTS (SELECT 1 FROM kb.scheduled_jobs WHERE job_type = 'maintain_bm25_indexes');

-- +goose Down
DELETE FROM kb.scheduled_jobs WHERE job_type = 'maintain_bm25_indexes';
