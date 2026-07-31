-- +goose Up
-- Governed mappings between a governed term and either another governed term or
-- an external IRI. mapping strengths are the DR13-conservative set -- exact,
-- close, broad, narrow, related -- so lexical similarity can never be recorded
-- as equivalence. Exact mappings require explicit approval (approval_status),
-- per spec §9.1's "exact mappings require explicit approval".
CREATE TABLE IF NOT EXISTS kb.ontology_mappings (
    id              BIGSERIAL PRIMARY KEY,
    mapping_id      TEXT NOT NULL,
    version         INT NOT NULL DEFAULT 1,
    from_term_id    TEXT NOT NULL,
    to_term_id      TEXT,
    to_iri          TEXT,
    relation        TEXT NOT NULL CHECK (relation IN (
                        'exact', 'close', 'broad', 'narrow', 'related'
                    )),
    evidence        JSONB,
    approval_status TEXT NOT NULL DEFAULT 'pending' CHECK (approval_status IN (
                        'pending', 'approved', 'rejected'
                    )),
    module_id       TEXT NOT NULL,
    status          TEXT NOT NULL DEFAULT 'draft' CHECK (status IN (
                        'draft', 'in_review', 'approved', 'included_in_release',
                        'superseded', 'rejected'
                    )),
    create_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    create_by       TEXT,
    modify_time     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_by       TEXT,
    CHECK (to_term_id IS NOT NULL OR to_iri IS NOT NULL)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_kb_ontology_mappings_id_version
    ON kb.ontology_mappings (mapping_id, version);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_mappings_from
    ON kb.ontology_mappings (from_term_id);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_mappings_to
    ON kb.ontology_mappings (to_term_id);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_mappings;
