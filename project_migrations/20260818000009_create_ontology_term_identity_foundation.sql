-- +goose Up
-- ADR 2026081701 phase 1: keep the legacy versioned table authoritative while
-- introducing an additive stable-header/revision representation. Existing
-- foreign keys continue to reference kb.ontology_terms(id); the compatibility
-- view exposes the latest append-only revision with the same row identity.

CREATE TABLE IF NOT EXISTS kb.ontology_term_headers (
    term_id         TEXT PRIMARY KEY,
    term_kind       TEXT NOT NULL,
    module_id       TEXT NOT NULL,
    create_time     TIMESTAMPTZ NOT NULL,
    create_by       TEXT
);

CREATE INDEX IF NOT EXISTS idx_kb_ontology_term_headers_module_kind
    ON kb.ontology_term_headers (module_id, term_kind);

CREATE TABLE IF NOT EXISTS kb.ontology_term_revisions (
    id                          BIGSERIAL PRIMARY KEY,
    term_id                     TEXT NOT NULL REFERENCES kb.ontology_term_headers (term_id),
    revision                    INT NOT NULL,
    source_term_row_id          BIGINT NOT NULL UNIQUE REFERENCES kb.ontology_terms (id),
    term_kind                   TEXT NOT NULL,
    module_id                   TEXT NOT NULL,
    status                      TEXT NOT NULL,
    definition                  TEXT,
    scope                       TEXT,
    value_type                  TEXT,
    range_type                  TEXT,
    permitted_unit_term_ids     JSONB,
    source_candidate_id         BIGINT,
    released_in_release_id      BIGINT,
    create_time                 TIMESTAMPTZ NOT NULL,
    create_by                   TEXT,
    modify_time                 TIMESTAMPTZ NOT NULL,
    modify_by                   TEXT,
    UNIQUE (term_id, revision)
);

CREATE INDEX IF NOT EXISTS idx_kb_ontology_term_revisions_current_lookup
    ON kb.ontology_term_revisions (term_id, revision DESC);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_term_revisions_module_status
    ON kb.ontology_term_revisions (module_id, status);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_term_revisions_release
    ON kb.ontology_term_revisions (released_in_release_id);

-- The header is seeded from each term's earliest preserved legacy version so
-- its creation provenance stays stable even as later revisions are appended.
INSERT INTO kb.ontology_term_headers (
    term_id,
    term_kind,
    module_id,
    create_time,
    create_by
)
SELECT DISTINCT ON (term_id)
    term_id,
    term_kind,
    module_id,
    create_time,
    create_by
FROM kb.ontology_terms
ORDER BY term_id, version ASC, id ASC
ON CONFLICT (term_id) DO NOTHING;

-- Copy every legacy version once. source_term_row_id maintains a direct,
-- auditable link to all pre-foundation term references.
INSERT INTO kb.ontology_term_revisions (
    term_id,
    revision,
    source_term_row_id,
    term_kind,
    module_id,
    status,
    definition,
    scope,
    value_type,
    range_type,
    permitted_unit_term_ids,
    source_candidate_id,
    released_in_release_id,
    create_time,
    create_by,
    modify_time,
    modify_by
)
SELECT
    term_id,
    version,
    id,
    term_kind,
    module_id,
    status,
    definition,
    scope,
    value_type,
    range_type,
    permitted_unit_term_ids,
    source_candidate_id,
    released_in_release_id,
    create_time,
    create_by,
    modify_time,
    modify_by
FROM kb.ontology_terms
ON CONFLICT (source_term_row_id) DO NOTHING;

-- The legacy writer remains available during the compatibility period. Mirror
-- every new legacy version into the append-only representation so readers of
-- the current view never silently miss newly authored terms.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.sync_ontology_term_revision_after_insert()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    INSERT INTO kb.ontology_term_headers (
        term_id,
        term_kind,
        module_id,
        create_time,
        create_by
    )
    VALUES (
        NEW.term_id,
        NEW.term_kind,
        NEW.module_id,
        NEW.create_time,
        NEW.create_by
    )
    ON CONFLICT (term_id) DO NOTHING;

    INSERT INTO kb.ontology_term_revisions (
        term_id,
        revision,
        source_term_row_id,
        term_kind,
        module_id,
        status,
        definition,
        scope,
        value_type,
        range_type,
        permitted_unit_term_ids,
        source_candidate_id,
        released_in_release_id,
        create_time,
        create_by,
        modify_time,
        modify_by
    )
    VALUES (
        NEW.term_id,
        NEW.version,
        NEW.id,
        NEW.term_kind,
        NEW.module_id,
        NEW.status,
        NEW.definition,
        NEW.scope,
        NEW.value_type,
        NEW.range_type,
        NEW.permitted_unit_term_ids,
        NEW.source_candidate_id,
        NEW.released_in_release_id,
        NEW.create_time,
        NEW.create_by,
        NEW.modify_time,
        NEW.modify_by
    )
    ON CONFLICT (source_term_row_id) DO NOTHING;

    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS kb_sync_ontology_term_revision_after_insert ON kb.ontology_terms;
CREATE TRIGGER kb_sync_ontology_term_revision_after_insert
AFTER INSERT ON kb.ontology_terms
FOR EACH ROW
EXECUTE FUNCTION kb.sync_ontology_term_revision_after_insert();

-- Current-state readers can migrate to this view without changing selected
-- columns or invalidating the legacy row id they expose to callers.
CREATE OR REPLACE VIEW kb.ontology_terms_current AS
SELECT DISTINCT ON (revision.term_id)
    revision.source_term_row_id AS id,
    revision.term_id,
    revision.revision AS version,
    revision.term_kind,
    revision.module_id,
    revision.status,
    revision.definition,
    revision.scope,
    revision.source_candidate_id,
    revision.create_time,
    revision.create_by,
    revision.modify_time,
    revision.modify_by,
    revision.released_in_release_id,
    revision.value_type,
    revision.range_type,
    revision.permitted_unit_term_ids
FROM kb.ontology_term_revisions AS revision
ORDER BY revision.term_id, revision.revision DESC, revision.id DESC;

-- +goose Down
DROP VIEW IF EXISTS kb.ontology_terms_current;
DROP TRIGGER IF EXISTS kb_sync_ontology_term_revision_after_insert ON kb.ontology_terms;
DROP FUNCTION IF EXISTS kb.sync_ontology_term_revision_after_insert();
DROP TABLE IF EXISTS kb.ontology_term_revisions;
DROP TABLE IF EXISTS kb.ontology_term_headers;
