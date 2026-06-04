-- +goose Up
ALTER TABLE kb.scene_objects DROP COLUMN IF EXISTS source_refs;

-- +goose Down
ALTER TABLE kb.scene_objects ADD COLUMN IF NOT EXISTS source_refs JSONB NOT NULL DEFAULT '[]'::jsonb;
