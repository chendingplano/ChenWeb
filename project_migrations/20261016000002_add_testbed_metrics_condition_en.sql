-- +goose Up
-- English rendering of the gold condition for non-English documents, matching
-- kb.metrics.condition_en (openspec change extract-metrics-conditions).
ALTER TABLE testbed.metrics
    ADD COLUMN IF NOT EXISTS condition_en TEXT;

-- +goose Down
ALTER TABLE testbed.metrics
    DROP COLUMN IF EXISTS condition_en;
