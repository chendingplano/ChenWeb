-- +goose Up
CREATE TABLE IF NOT EXISTS kb.pipeline_bindings (
    id BIGSERIAL PRIMARY KEY,
    ks_store_id BIGINT NOT NULL REFERENCES kb.knowledge_store(id) ON DELETE CASCADE,
    pipeline_id BIGINT NOT NULL REFERENCES kb.pipelines(id) ON DELETE RESTRICT,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (ks_store_id)
);

CREATE INDEX IF NOT EXISTS idx_kb_pipeline_bindings_pipeline_id
    ON kb.pipeline_bindings (pipeline_id);

-- default_pipeline was a free-text column on kb.knowledge_store (added
-- 20260731000003) with no referential integrity to kb.pipelines and no
-- API-level validation. kb.pipeline_bindings replaces it as the authored
-- store->pipeline binding the ADR names: one row per store, FK-enforced
-- against kb.pipelines, so an unknown pipeline can no longer be bound to a
-- store at all (previously it only failed later, at plan-resolution time).
ALTER TABLE kb.knowledge_store
    DROP COLUMN IF EXISTS default_pipeline;

-- +goose Down
ALTER TABLE kb.knowledge_store
    ADD COLUMN IF NOT EXISTS default_pipeline TEXT;

DROP TABLE IF EXISTS kb.pipeline_bindings;
