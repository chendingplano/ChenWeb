-- +goose Up
-- Version of the extract-metrics-benchmark skill that produced each benchmark row.
-- Rows saved before skill versioning existed were produced by skill version 1.0.0.
ALTER TABLE testbed.metrics ADD COLUMN IF NOT EXISTS skill_version TEXT;
UPDATE testbed.metrics SET skill_version = '1.0.0' WHERE skill_version IS NULL;
ALTER TABLE testbed.metrics ALTER COLUMN skill_version SET NOT NULL;

-- +goose Down
ALTER TABLE testbed.metrics DROP COLUMN IF EXISTS skill_version;
