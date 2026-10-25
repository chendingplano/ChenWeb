-- +goose Up
-- Number of questions a benchmark run searches at the same time (earlier runs: one at a time).
ALTER TABLE testbed.hybrid_search_benchmark_runs
    ADD COLUMN IF NOT EXISTS parallelism INTEGER NOT NULL DEFAULT 1 CHECK (parallelism BETWEEN 1 AND 64);

-- +goose Down
ALTER TABLE testbed.hybrid_search_benchmark_runs
    DROP COLUMN IF EXISTS parallelism;
