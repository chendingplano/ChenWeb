-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.knowledge_store (
    id BIGSERIAL PRIMARY KEY,
    tenant_id VARCHAR(128) NOT NULL DEFAULT '-',
    ks_type VARCHAR(128),
    ks_name VARCHAR(256) NOT NULL,
    ks_desc TEXT,
    ks_sync_mode VARCHAR(32) NOT NULL DEFAULT 'manual'
        CHECK (ks_sync_mode IN ('auto', 'manual')),
    ks_sources TEXT[] NOT NULL DEFAULT '{}'::text[],
    status VARCHAR(32) NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'suspended', 'inactive')),
    notes TEXT,
    error_msg TEXT,
    public_info JSONB NOT NULL DEFAULT '{}'::jsonb,
    private_info JSONB NOT NULL DEFAULT '{}'::jsonb,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_knowledge_store_tenant_id
    ON kb.knowledge_store(tenant_id);

CREATE INDEX IF NOT EXISTS idx_kb_knowledge_store_status
    ON kb.knowledge_store(status);

CREATE INDEX IF NOT EXISTS idx_kb_knowledge_store_modify_time
    ON kb.knowledge_store(modify_time DESC);

-- +goose Down
DROP TABLE IF EXISTS kb.knowledge_store;
