-- +goose Up
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS connected_artifacts;
