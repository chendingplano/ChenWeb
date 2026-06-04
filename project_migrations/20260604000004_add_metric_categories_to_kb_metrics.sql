-- +goose Up
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS metric_categories    text NOT NULL DEFAULT '';
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS metric_categories_en text;

-- +goose Down
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS metric_categories_en;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS metric_categories;
