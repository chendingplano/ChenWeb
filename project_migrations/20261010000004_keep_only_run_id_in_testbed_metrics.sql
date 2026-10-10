-- +goose Up
-- Replace the generated alias with the original stored column under its canonical
-- name. PostgreSQL updates the existing unique/check constraints, index and run
-- foreign key when renaming, so run associations and values remain intact.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_schema = 'testbed' AND table_name = 'metrics'
                 AND column_name = 'benchmark_run_id') THEN
        ALTER TABLE testbed.metrics DROP COLUMN IF EXISTS run_id;
        ALTER TABLE testbed.metrics RENAME COLUMN benchmark_run_id TO run_id;
    END IF;
END $$;
-- +goose StatementEnd
COMMENT ON COLUMN testbed.metrics.run_id IS
    'Benchmark run ID (yyyymmdd_hhmmss). Run identity also includes skill_name, skill_version and model_name.';

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM information_schema.columns
                   WHERE table_schema = 'testbed' AND table_name = 'metrics'
                     AND column_name = 'benchmark_run_id') THEN
        ALTER TABLE testbed.metrics RENAME COLUMN run_id TO benchmark_run_id;
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE testbed.metrics ADD COLUMN IF NOT EXISTS run_id TEXT
    GENERATED ALWAYS AS (benchmark_run_id) STORED NOT NULL;
COMMENT ON COLUMN testbed.metrics.run_id IS
    'Benchmark run ID (yyyymmdd_hhmmss), generated from benchmark_run_id. Run identity also includes skill_name, skill_version and model_name.';
