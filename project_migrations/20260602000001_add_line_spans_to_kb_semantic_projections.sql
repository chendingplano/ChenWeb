-- +goose Up
ALTER TABLE kb.semantic_projections ADD COLUMN IF NOT EXISTS line_spans JSONB;

-- +goose Down
ALTER TABLE kb.semantic_projections DROP COLUMN IF EXISTS line_spans;
