-- +goose Up
CREATE TABLE IF NOT EXISTS kb.doc_facets (
    record_id BIGINT PRIMARY KEY REFERENCES kb.inputs(id) ON DELETE CASCADE,
    ks_store_id BIGINT NOT NULL DEFAULT 0,
    knowledge_store_binding VARCHAR(32) NOT NULL DEFAULT 'absent',
    input_doc_type VARCHAR(128) NOT NULL DEFAULT '',
    source_language VARCHAR(32) NOT NULL DEFAULT '',
    has_document_number BOOLEAN NOT NULL DEFAULT false,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_doc_facets_ks_store_id
    ON kb.doc_facets (ks_store_id);

CREATE INDEX IF NOT EXISTS idx_kb_doc_facets_input_doc_type
    ON kb.doc_facets (input_doc_type);

-- +goose Down
DROP TABLE IF EXISTS kb.doc_facets;
