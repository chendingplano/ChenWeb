-- +goose Up
ALTER TABLE kb.artifact_categories
    ADD COLUMN IF NOT EXISTS parent_categories JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN IF NOT EXISTS related_categories JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE kb.artifact_categories
    DROP COLUMN IF EXISTS related_categories,
    DROP COLUMN IF EXISTS parent_categories;
