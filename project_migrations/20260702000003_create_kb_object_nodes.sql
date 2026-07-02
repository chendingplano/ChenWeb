-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;
CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS kb.object_nodes (
    id                  BIGSERIAL PRIMARY KEY,
    object_id           TEXT        NOT NULL UNIQUE,
    canonical_object_id TEXT,
    canonical_name      TEXT        NOT NULL DEFAULT '',
    canonical_name_en   TEXT,
    canonical_name_zh   TEXT,
    primary_language    TEXT,
    object_type         TEXT        NOT NULL DEFAULT 'other',
    aliases             JSONB       NOT NULL DEFAULT '[]'::jsonb,
    acronyms            JSONB       NOT NULL DEFAULT '[]'::jsonb,
    normalized_names    JSONB       NOT NULL DEFAULT '[]'::jsonb,
    description         TEXT,
    search_document     TEXT        NOT NULL DEFAULT '',
    embedding           vector(1536),
    reconcile_status    TEXT        NOT NULL DEFAULT 'active',
    ext_info            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    create_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_kb_object_nodes_reconcile_status
        CHECK (reconcile_status IN ('active', 'merged', 'pending_review', 'rejected'))
);

CREATE INDEX IF NOT EXISTS idx_kb_object_nodes_canonical_object_id
    ON kb.object_nodes (canonical_object_id);
CREATE INDEX IF NOT EXISTS idx_kb_object_nodes_type_name
    ON kb.object_nodes (object_type, canonical_name);
CREATE INDEX IF NOT EXISTS idx_kb_object_nodes_normalized_names
    ON kb.object_nodes USING GIN (normalized_names);
CREATE INDEX IF NOT EXISTS idx_kb_object_nodes_search_vector
    ON kb.object_nodes USING GIN (to_tsvector('simple', COALESCE(search_document, '')));

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.touch_object_nodes_modify_time()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.modify_time := NOW();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_touch_object_nodes_modify_time ON kb.object_nodes;
CREATE TRIGGER trg_touch_object_nodes_modify_time
BEFORE UPDATE ON kb.object_nodes
FOR EACH ROW
EXECUTE FUNCTION kb.touch_object_nodes_modify_time();

-- +goose Down
DROP TRIGGER IF EXISTS trg_touch_object_nodes_modify_time ON kb.object_nodes;
DROP FUNCTION IF EXISTS kb.touch_object_nodes_modify_time();
DROP INDEX IF EXISTS kb.idx_kb_object_nodes_search_vector;
DROP INDEX IF EXISTS kb.idx_kb_object_nodes_normalized_names;
DROP INDEX IF EXISTS kb.idx_kb_object_nodes_type_name;
DROP INDEX IF EXISTS kb.idx_kb_object_nodes_canonical_object_id;
DROP TABLE IF EXISTS kb.object_nodes;
