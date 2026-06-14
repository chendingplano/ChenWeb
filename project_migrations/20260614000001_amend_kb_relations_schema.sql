-- +goose Up
-- Remove source_line_spans from kb.relations (field was in the spec but never
-- added to the table; relations use line_spans derived from endpoint spans).
ALTER TABLE kb.relations DROP COLUMN IF EXISTS source_line_spans;

-- Add categories to kb.relations (populated from relation_categories returned
-- by the Phase 2 LLM call via prompt-extract-relations-v1.md).
ALTER TABLE kb.relations ADD COLUMN IF NOT EXISTS categories JSONB;

-- +goose Down
ALTER TABLE kb.relations DROP COLUMN IF EXISTS categories;
