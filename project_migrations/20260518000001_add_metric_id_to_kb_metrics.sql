-- +goose Up
ALTER TABLE kb.metrics
    ADD COLUMN IF NOT EXISTS metric_id TEXT;

-- +goose Down
ALTER TABLE kb.metrics
    DROP COLUMN IF EXISTS metric_id;
