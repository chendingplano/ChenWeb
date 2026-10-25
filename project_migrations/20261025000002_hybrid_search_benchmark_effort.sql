-- +goose Up
-- Search effort level of a benchmark run and its attempt limit (0 for low).
ALTER TABLE testbed.hybrid_search_benchmark_runs
    ADD COLUMN IF NOT EXISTS effort TEXT NOT NULL DEFAULT 'low' CHECK (effort IN ('low', 'medium', 'high')),
    ADD COLUMN IF NOT EXISTS max_efforts INTEGER NOT NULL DEFAULT 0 CHECK (max_efforts >= 0);

-- Per-question result of the effort loop (medium/high runs only): the target's rank in the final
-- list (NULL = miss), whether the judge found a match, attempts made, time and the trace.
ALTER TABLE testbed.hybrid_search_benchmark_results
    ADD COLUMN IF NOT EXISTS effort_rank INTEGER CHECK (effort_rank > 0),
    ADD COLUMN IF NOT EXISTS effort_found BOOLEAN,
    ADD COLUMN IF NOT EXISTS effort_attempts INTEGER,
    ADD COLUMN IF NOT EXISTS effort_ms INTEGER,
    ADD COLUMN IF NOT EXISTS effort_trace JSONB;

-- +goose Down
ALTER TABLE testbed.hybrid_search_benchmark_results
    DROP COLUMN IF EXISTS effort_trace,
    DROP COLUMN IF EXISTS effort_ms,
    DROP COLUMN IF EXISTS effort_attempts,
    DROP COLUMN IF EXISTS effort_found,
    DROP COLUMN IF EXISTS effort_rank;
ALTER TABLE testbed.hybrid_search_benchmark_runs
    DROP COLUMN IF EXISTS max_efforts,
    DROP COLUMN IF EXISTS effort;
