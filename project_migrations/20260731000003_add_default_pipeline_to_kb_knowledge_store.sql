-- +goose Up
ALTER TABLE kb.knowledge_store
    ADD COLUMN IF NOT EXISTS default_pipeline TEXT;

-- +goose Down
ALTER TABLE kb.knowledge_store
    DROP COLUMN IF EXISTS default_pipeline;
