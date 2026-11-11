-- +goose Up
-- The artifact type a run searched: one search partition, or 'all' (mixed). Earlier runs searched metrics.
ALTER TABLE testbed.hybrid_search_benchmark_runs
    ADD COLUMN IF NOT EXISTS search_type TEXT NOT NULL DEFAULT 'metric';

-- +goose Down
ALTER TABLE testbed.hybrid_search_benchmark_runs DROP COLUMN IF EXISTS search_type;
