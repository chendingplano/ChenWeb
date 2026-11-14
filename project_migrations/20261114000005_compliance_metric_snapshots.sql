-- +goose Up
-- Existing snapshots retain their original chunk/provision scope. New writers explicitly use v2.
ALTER TABLE testbed.compliance_eval_snapshots
    ADD COLUMN IF NOT EXISTS manifest_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE testbed.compliance_eval_snapshot_records
    ADD COLUMN IF NOT EXISTS metric_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS metric_fingerprint TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE testbed.compliance_eval_snapshot_records
    DROP COLUMN IF EXISTS metric_fingerprint,
    DROP COLUMN IF EXISTS metric_count;
ALTER TABLE testbed.compliance_eval_snapshots
    DROP COLUMN IF EXISTS manifest_version;
