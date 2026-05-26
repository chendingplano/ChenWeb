-- +goose Up
ALTER TABLE kb.semantic_projections
    ADD COLUMN IF NOT EXISTS semantic_projection TEXT,
    ADD COLUMN IF NOT EXISTS semantic_projection_en TEXT;

-- +goose Down
ALTER TABLE kb.semantic_projections
    DROP COLUMN IF EXISTS semantic_projection,
    DROP COLUMN IF EXISTS semantic_projection_en;
