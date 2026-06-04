-- +goose Up
ALTER TABLE kb.scene_objects ADD COLUMN IF NOT EXISTS line_spans JSONB NOT NULL DEFAULT '[]'::jsonb;

-- +goose Down
ALTER TABLE kb.scene_objects DROP COLUMN IF EXISTS line_spans;
