-- +goose Up
--
-- Separate "editable save revision" from visible document content versions,
-- and persist one snapshot row per visible content version so the frontend
-- can show version history and support "save current version" vs.
-- "save to new version".

ALTER TABLE kb.cdm_documents
    ADD COLUMN IF NOT EXISTS edit_version BIGINT NOT NULL DEFAULT 1;

CREATE TABLE IF NOT EXISTS kb.cdm_document_versions (
    id                     BIGSERIAL PRIMARY KEY,
    document_id            BIGINT NOT NULL
                               REFERENCES kb.cdm_documents(id) ON DELETE CASCADE,
    content_version        BIGINT NOT NULL,
    parent_content_version BIGINT,
    semantic_document      JSONB NOT NULL,
    size_bytes             BIGINT NOT NULL,
    create_time            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    update_time            TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_kb_cdm_document_versions
        UNIQUE (document_id, content_version)
);

CREATE INDEX IF NOT EXISTS idx_kb_cdm_document_versions_document_id
    ON kb.cdm_document_versions(document_id, content_version DESC);

INSERT INTO kb.cdm_document_versions
    (document_id, content_version, parent_content_version, semantic_document, size_bytes, create_time, update_time)
SELECT d.id,
       d.content_version,
       CASE WHEN d.content_version > 1 THEN d.content_version - 1 ELSE NULL END,
       d.semantic_document,
       octet_length(convert_to(d.semantic_document::text, 'UTF8')),
       d.create_time,
       d.update_time
FROM kb.cdm_documents d
ON CONFLICT (document_id, content_version) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS kb.cdm_document_versions;
ALTER TABLE kb.cdm_documents DROP COLUMN IF EXISTS edit_version;
