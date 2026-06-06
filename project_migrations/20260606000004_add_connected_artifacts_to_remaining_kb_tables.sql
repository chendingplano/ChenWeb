-- +goose Up
ALTER TABLE kb.scene_objects
    ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE kb.summaries
    ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE kb.topics
    ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE kb.provisions
    ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE kb.entities
    ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;

ALTER TABLE kb.relations
    ADD COLUMN IF NOT EXISTS connected_artifacts JSONB NOT NULL DEFAULT '{}'::jsonb;

-- +goose Down
ALTER TABLE kb.relations
    DROP COLUMN IF EXISTS connected_artifacts;

ALTER TABLE kb.entities
    DROP COLUMN IF EXISTS connected_artifacts;

ALTER TABLE kb.provisions
    DROP COLUMN IF EXISTS connected_artifacts;

ALTER TABLE kb.topics
    DROP COLUMN IF EXISTS connected_artifacts;

ALTER TABLE kb.summaries
    DROP COLUMN IF EXISTS connected_artifacts;

ALTER TABLE kb.scene_objects
    DROP COLUMN IF EXISTS connected_artifacts;
