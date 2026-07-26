-- +goose Up
--
-- Canonical Document Model (CDM) v1.0, Phase 1 storage.
--
-- See KnowledgeStore/doc-repo/specs/202607/2026072501-spec-canonical-doc-model.md
-- §11, and ADR 2026072501 DR12: these tables use the kb.cdm_* namespace
-- (not kb.documents / kb.document_projections) specifically to avoid
-- collision with kb.semantic_projections below, and with the crowded
-- kb.doc_* family.
--
-- kb.cdm_documents.semantic_document is authoritative; kb.cdm_blocks is a
-- derived flattening rewritten transactionally on every save (spec §11,
-- design D5). Block references throughout are TEXT slugs, never uuid
-- (ADR 2026072501 DR1).

CREATE TABLE IF NOT EXISTS kb.cdm_documents (
    id                BIGSERIAL PRIMARY KEY,
    document_key      VARCHAR(255) NOT NULL UNIQUE,
    title             TEXT NOT NULL,
    language          VARCHAR(32),
    schema_version    VARCHAR(32) NOT NULL,
    content_version   BIGINT NOT NULL DEFAULT 1,
    doc_type          VARCHAR(64),
    rendering_type    VARCHAR(64),
    authors           TEXT[] NOT NULL DEFAULT '{}',
    doc_version       VARCHAR(32),
    input_record_id   BIGINT REFERENCES kb.inputs(id) ON DELETE SET NULL,
    semantic_document JSONB NOT NULL,
    create_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    update_time       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE kb.cdm_documents IS
    'Canonical Document Model (CDM) documents authored inside SemOS. '
    'semantic_document is authoritative; see spec 2026072501-spec-canonical-doc-model.';

CREATE INDEX IF NOT EXISTS idx_kb_cdm_documents_input_record_id
    ON kb.cdm_documents(input_record_id);

CREATE TABLE IF NOT EXISTS kb.cdm_blocks (
    id                BIGSERIAL PRIMARY KEY,
    document_id       BIGINT NOT NULL
                          REFERENCES kb.cdm_documents(id) ON DELETE CASCADE,
    block_id          VARCHAR(255) NOT NULL,
    parent_block_id   VARCHAR(255),
    block_type        VARCHAR(64) NOT NULL,
    block_role        VARCHAR(64),
    ordinal           INTEGER NOT NULL,
    section_path      TEXT[],
    semantic_content  JSONB NOT NULL,
    source_provenance JSONB,
    content_hash      VARCHAR(64) NOT NULL,
    create_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    update_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_kb_cdm_blocks_block_id
        UNIQUE (document_id, block_id),
    CONSTRAINT uq_kb_cdm_blocks_ordinal
        UNIQUE NULLS NOT DISTINCT (document_id, parent_block_id, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_kb_cdm_blocks_document_id
    ON kb.cdm_blocks(document_id);

CREATE TABLE IF NOT EXISTS kb.cdm_renderings (
    id                BIGSERIAL PRIMARY KEY,
    document_id       BIGINT NOT NULL
                          REFERENCES kb.cdm_documents(id) ON DELETE CASCADE,
    content_version   BIGINT NOT NULL,
    renderer          VARCHAR(32) NOT NULL,
    renderer_version  VARCHAR(32) NOT NULL,
    media_type        VARCHAR(128) NOT NULL,
    rendered_content  BYTEA NOT NULL,
    create_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_kb_cdm_renderings
        UNIQUE (document_id, content_version, renderer, renderer_version)
);

CREATE TABLE IF NOT EXISTS kb.cdm_projections (
    id                 BIGSERIAL PRIMARY KEY,
    document_id        BIGINT NOT NULL
                           REFERENCES kb.cdm_documents(id) ON DELETE CASCADE,
    content_version    BIGINT NOT NULL,
    chunk_id           VARCHAR(255) NOT NULL,
    block_ids          TEXT[] NOT NULL,
    projection_type    VARCHAR(32) NOT NULL,
    projection_version VARCHAR(32) NOT NULL,
    section_path       TEXT[],
    semantic_type      VARCHAR(64),
    projected_text     TEXT NOT NULL,
    metadata           JSONB,
    embedding          vector(1536),
    create_time        TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_kb_cdm_projections
        UNIQUE (document_id, content_version, projection_type,
                projection_version, chunk_id)
);

COMMENT ON TABLE kb.cdm_projections IS
    'CDM retrieval projections (Phase 2; unused in Phase 1). NOT related to '
    'kb.semantic_projections, which is a doc-processor search enrichment of '
    'an uploaded input. A CDM projection is a de-formatted textual rendition '
    'of a CDM document, not a summary. See ADR 2026072501 DR12.';

CREATE INDEX IF NOT EXISTS idx_kb_cdm_projections_document_id
    ON kb.cdm_projections(document_id);

CREATE TABLE IF NOT EXISTS kb.cdm_anchors (
    id                BIGSERIAL PRIMARY KEY,
    document_id       BIGINT NOT NULL
                          REFERENCES kb.cdm_documents(id) ON DELETE CASCADE,
    content_version   BIGINT NOT NULL,
    renderer_version  VARCHAR(32) NOT NULL,
    line_number       INTEGER NOT NULL,
    block_id          VARCHAR(255) NOT NULL,
    fragment_ordinal  INTEGER NOT NULL DEFAULT 0,
    page              INTEGER NOT NULL,
    x                 REAL NOT NULL,
    y                 REAL NOT NULL,
    w                 REAL NOT NULL,
    h                 REAL NOT NULL,
    create_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT uq_kb_cdm_anchors
        UNIQUE (document_id, content_version, renderer_version,
                line_number, fragment_ordinal)
);

COMMENT ON TABLE kb.cdm_anchors IS
    'Per-line-file-unit page coordinates for CDM anchored rendering '
    '(spec §5.7): the location substrate that replaces PDF bboxes for '
    'authored documents. See ADR 2026072601.';

CREATE INDEX IF NOT EXISTS idx_kb_cdm_anchors_lookup
    ON kb.cdm_anchors(document_id, content_version, line_number);

COMMENT ON TABLE kb.semantic_projections IS
    'Doc-processor search enrichment of an uploaded kb.inputs record '
    '(keywords, category paths, search vectors). NOT related to '
    'kb.cdm_projections, which is a CDM document rendition. '
    'See ADR 2026072501 DR12.';

-- +goose Down
COMMENT ON TABLE kb.semantic_projections IS NULL;
DROP TABLE IF EXISTS kb.cdm_anchors;
DROP TABLE IF EXISTS kb.cdm_projections;
DROP TABLE IF EXISTS kb.cdm_renderings;
DROP TABLE IF EXISTS kb.cdm_blocks;
DROP TABLE IF EXISTS kb.cdm_documents;
