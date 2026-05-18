-- +goose Up
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS category_paths JSONB;
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS category_paths_en JSONB;

-- +goose Down
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS category_paths;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS category_paths_en;
