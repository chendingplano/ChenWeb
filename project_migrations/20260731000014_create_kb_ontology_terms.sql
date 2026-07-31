-- +goose Up
-- Governed ontology terms, versioned per the 2026-07-31 storage decision:
-- data lives in the database, versioning is by column, and an accepted change
-- inserts a new version row (the latest version is the current state).
--
-- term_id is the immutable, namespaced identity (e.g. `core:assertion`); a
-- material meaning change creates a replacement term rather than redefining it.
-- `status` follows the governed ontology-content lifecycle (spec §9.3); the
-- `included_in_release` transition and the release linkage are added in chunk B
-- alongside kb.ontology_module_releases. `module_id` gains referential integrity
-- to kb.ontology_modules in the same chunk.
CREATE TABLE IF NOT EXISTS kb.ontology_terms (
    id              BIGSERIAL PRIMARY KEY,
    term_id         TEXT NOT NULL,
    version         INT NOT NULL DEFAULT 1,
    term_kind       TEXT NOT NULL CHECK (term_kind IN (
                        'class', 'property', 'individual', 'concept',
                        'metric_definition', 'quantity_kind', 'unit', 'dimension'
                    )),
    module_id       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN (
                        'draft', 'in_review', 'approved', 'included_in_release',
                        'superseded', 'rejected'
                    )),
    definition      TEXT,
    scope           TEXT,
    source_candidate_id BIGINT,
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by       TEXT,
    modify_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by       TEXT
);

-- Version identity: one row per (term_id, version); latest version is current.
CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_ontology_terms_term_version
    ON kb.ontology_terms (term_id, version);

-- Lookups that route the canonicalization kernel and future releases.
CREATE INDEX IF NOT EXISTS idx_kb_ontology_terms_module_status
    ON kb.ontology_terms (module_id, status);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_terms_status
    ON kb.ontology_terms (status);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_terms;
