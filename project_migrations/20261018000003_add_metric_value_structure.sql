-- +goose Up
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS value_structure JSONB;
ALTER TABLE testbed.metrics ADD COLUMN IF NOT EXISTS value_structure JSONB;

-- +goose Down
ALTER TABLE testbed.metrics DROP COLUMN IF EXISTS value_structure;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS value_structure;
