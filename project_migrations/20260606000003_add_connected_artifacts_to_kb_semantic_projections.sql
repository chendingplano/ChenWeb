-- +goose Up
ALTER TABLE kb.semantic_projections
    ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE kb.semantic_projections
    DROP COLUMN IF EXISTS connected_artifacts;
