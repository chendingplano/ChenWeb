-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_inputs_id_store
    ON kb.inputs(id, ks_store_id);

CREATE TABLE IF NOT EXISTS kb.agentic_knowledge_grants (
    id                 BIGSERIAL PRIMARY KEY,
    user_id            TEXT NOT NULL,
    knowledge_store_id BIGINT NOT NULL REFERENCES kb.knowledge_store(id) ON DELETE CASCADE,
    document_id        BIGINT REFERENCES kb.inputs(id) ON DELETE CASCADE,
    active             BOOLEAN NOT NULL DEFAULT TRUE,
    expires_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agentic_grant_document_store_fk
        FOREIGN KEY (document_id, knowledge_store_id)
        REFERENCES kb.inputs(id, ks_store_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_agentic_knowledge_store_grant
    ON kb.agentic_knowledge_grants(user_id, knowledge_store_id)
    WHERE document_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS uq_agentic_knowledge_document_grant
    ON kb.agentic_knowledge_grants(user_id, knowledge_store_id, document_id)
    WHERE document_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_agentic_knowledge_grants_lookup
    ON kb.agentic_knowledge_grants(user_id, knowledge_store_id, active);

-- +goose Down
DROP TABLE IF EXISTS kb.agentic_knowledge_grants;
DROP INDEX IF EXISTS kb.uq_kb_inputs_id_store;
