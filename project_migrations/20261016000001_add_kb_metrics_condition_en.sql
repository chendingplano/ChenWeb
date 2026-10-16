-- +goose Up
-- English rendering of kb.metrics.condition for non-English documents, like the
-- other _en columns (openspec change extract-metrics-conditions, D6).
ALTER TABLE kb.metrics
    ADD COLUMN IF NOT EXISTS condition_en TEXT;

-- +goose Down
ALTER TABLE kb.metrics
    DROP COLUMN IF EXISTS condition_en;
