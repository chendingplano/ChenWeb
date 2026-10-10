-- +goose Up
-- openspec change extract-references-processor (devdoc 2026101002). extract_references stores
-- every reference of a document in kb.ref_mentions; the resources they point at live in a
-- catalog that separates identity (kb.ref_resources) from editions (kb.ref_resource_versions)
-- and from every way of naming or locating them (kb.ref_resource_identifiers). visibility is
-- 'public' for resources known by a public identifier (shared across customers) or
-- 'store:<id>' / 'user:<id>' for resources known only from a private source. A document's own
-- copy is attached through an identifier of kind 'kb_input', never public.
-- kb.reference_artifact_links holds, per artifact family (metric, provision, ...), the candidate
-- and resolved target artifacts of a mention.
CREATE TABLE IF NOT EXISTS kb.ref_resources (
    id              BIGSERIAL    PRIMARY KEY,
    identity_key    TEXT         NOT NULL,
    visibility      TEXT         NOT NULL,
    target_type     TEXT         NOT NULL,
    target_subtype  TEXT         NOT NULL DEFAULT 'others',
    title           TEXT,
    title_en        TEXT,
    issuer          TEXT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (identity_key, visibility)
);

CREATE TABLE IF NOT EXISTS kb.ref_resource_versions (
    id                    BIGSERIAL    PRIMARY KEY,
    resource_id           BIGINT       NOT NULL REFERENCES kb.ref_resources(id) ON DELETE CASCADE,
    version_label         TEXT         NOT NULL,
    status                TEXT,
    supersedes_version_id BIGINT       REFERENCES kb.ref_resource_versions(id) ON DELETE SET NULL,
    adoption_degree       TEXT,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (resource_id, version_label)
);

CREATE TABLE IF NOT EXISTS kb.ref_resource_identifiers (
    id          BIGSERIAL    PRIMARY KEY,
    resource_id BIGINT       NOT NULL REFERENCES kb.ref_resources(id) ON DELETE CASCADE,
    version_id  BIGINT       REFERENCES kb.ref_resource_versions(id) ON DELETE CASCADE,
    kind        TEXT         NOT NULL,
    value       TEXT         NOT NULL,
    visibility  TEXT         NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (kind, value, visibility)
);

CREATE INDEX IF NOT EXISTS idx_kb_ref_resource_identifiers_resource ON kb.ref_resource_identifiers(resource_id);

CREATE TABLE IF NOT EXISTS kb.ref_mentions (
    id                   BIGSERIAL    PRIMARY KEY,
    input_record_id      BIGINT       NOT NULL,
    ref_id               TEXT         NOT NULL UNIQUE,
    event_id             TEXT,
    section              TEXT,
    source_line_spans    JSONB,
    evidence             TEXT,
    ref_form             TEXT         NOT NULL,
    list_kind            TEXT,
    list_entry_id        BIGINT       REFERENCES kb.ref_mentions(id) ON DELETE SET NULL,
    normativity          TEXT         NOT NULL,
    in_requirement       BOOLEAN      NOT NULL DEFAULT FALSE,
    list_mismatch        TEXT,
    mechanism            TEXT         NOT NULL,
    cited_raw            TEXT,
    identifiers          JSONB        NOT NULL DEFAULT '[]'::jsonb,
    target_type          TEXT         NOT NULL,
    target_subtype       TEXT         NOT NULL,
    target_subtype_label TEXT,
    target_title         TEXT,
    target_version       TEXT,
    relationships        TEXT[]       NOT NULL DEFAULT '{}',
    target_scope         TEXT         NOT NULL,
    target_locator       TEXT,
    target_content       TEXT         NOT NULL,
    metric_name          TEXT,
    metric_name_en       TEXT,
    metric_subject       TEXT,
    metric_subject_en    TEXT,
    condition            TEXT,
    internal             BOOLEAN      NOT NULL DEFAULT FALSE,
    confidence           DOUBLE PRECISION,
    resource_id          BIGINT       REFERENCES kb.ref_resources(id) ON DELETE SET NULL,
    version_id           BIGINT       REFERENCES kb.ref_resource_versions(id) ON DELETE SET NULL,
    r1_state             TEXT,
    target_input_id      BIGINT,
    r2_state             TEXT,
    target_section       TEXT,
    r3_state             TEXT,
    metric_state         TEXT,
    mention_key          TEXT         NOT NULL,
    ext_info             JSONB        NOT NULL DEFAULT '{}'::jsonb,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_ref_mentions_record_normativity ON kb.ref_mentions(input_record_id, normativity);
CREATE INDEX IF NOT EXISTS idx_kb_ref_mentions_resource ON kb.ref_mentions(resource_id);
CREATE INDEX IF NOT EXISTS idx_kb_ref_mentions_target_input ON kb.ref_mentions(target_input_id);
CREATE INDEX IF NOT EXISTS idx_kb_ref_mentions_target_type ON kb.ref_mentions(target_type);

CREATE TABLE IF NOT EXISTS kb.reference_artifact_links (
    id                    BIGSERIAL    PRIMARY KEY,
    ref_mention_id        BIGINT       NOT NULL REFERENCES kb.ref_mentions(id) ON DELETE CASCADE,
    artifact_family       TEXT         NOT NULL,
    citing_input_id       BIGINT       NOT NULL,
    target_input_id       BIGINT       NOT NULL,
    target_artifact_table TEXT         NOT NULL,
    target_artifact_id    TEXT         NOT NULL,
    target_artifact_key   TEXT         NOT NULL,
    rank                  INT          NOT NULL,
    score                 DOUBLE PRECISION NOT NULL,
    match_method          TEXT         NOT NULL,
    status                TEXT         NOT NULL,
    policy_id             BIGINT,
    policy_version        TEXT,
    decided_by            TEXT,
    decided_at            TIMESTAMPTZ,
    resolved_artifact_id  TEXT,
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (ref_mention_id, artifact_family, target_artifact_key)
);

CREATE INDEX IF NOT EXISTS idx_kb_reference_artifact_links_target ON kb.reference_artifact_links(target_input_id, artifact_family);
CREATE INDEX IF NOT EXISTS idx_kb_reference_artifact_links_citing ON kb.reference_artifact_links(citing_input_id);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_reference_artifact_links_citing;
DROP INDEX IF EXISTS kb.idx_kb_reference_artifact_links_target;
DROP TABLE IF EXISTS kb.reference_artifact_links;
DROP INDEX IF EXISTS kb.idx_kb_ref_mentions_target_type;
DROP INDEX IF EXISTS kb.idx_kb_ref_mentions_target_input;
DROP INDEX IF EXISTS kb.idx_kb_ref_mentions_resource;
DROP INDEX IF EXISTS kb.idx_kb_ref_mentions_record_normativity;
DROP TABLE IF EXISTS kb.ref_mentions;
DROP INDEX IF EXISTS kb.idx_kb_ref_resource_identifiers_resource;
DROP TABLE IF EXISTS kb.ref_resource_identifiers;
DROP TABLE IF EXISTS kb.ref_resource_versions;
DROP TABLE IF EXISTS kb.ref_resources;
