-- +goose Up
-- Expose the skill's existing run ID under the common run_id column name.
-- Generated storage covers existing rows and future saves without changing writers.
ALTER TABLE testbed.metrics ADD COLUMN IF NOT EXISTS run_id TEXT
    GENERATED ALWAYS AS (benchmark_run_id) STORED NOT NULL;
COMMENT ON COLUMN testbed.metrics.run_id IS
    'Benchmark run ID (yyyymmdd_hhmmss), generated from benchmark_run_id. Run identity also includes skill_name, skill_version and model_name.';

-- +goose Down
ALTER TABLE testbed.metrics DROP COLUMN IF EXISTS run_id;
