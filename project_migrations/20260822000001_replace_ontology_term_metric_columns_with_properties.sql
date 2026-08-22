-- +goose Up
-- Bug 2026082101: value_type/range_type/permitted_unit_term_ids
-- (ADR 2026081201 DR3) are metric_definition-only fields implemented as flat
-- columns on kb.ontology_terms, a table shared by 8 term kinds -- dead
-- weight for the other 7, and with no general home for kind-specific
-- structured properties (the QUDT importer has the same unmet need for its
-- symbol/deprecated payload). Replaced by a single `properties JSONB`
-- column, keyed by convention per term_kind. The auto-promotion path is the
-- only current producer of these columns and the data affected by the bug
-- has already been cleared, so this is a clean cut with no backfill.
--
-- Column drops must come last: kb.ontology_terms_current still SELECTs the
-- old columns until it is replaced, and Postgres refuses to drop a column a
-- live view depends on (2BP01).
ALTER TABLE kb.ontology_terms
    ADD COLUMN IF NOT EXISTS properties JSONB;
ALTER TABLE kb.ontology_term_revisions
    ADD COLUMN IF NOT EXISTS properties JSONB;

-- Mirror the column swap into the append-only sync trigger (ADR 2026081701).
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
        properties,
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
        NEW.properties,
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

-- Mirror the column swap into the compatibility view. CREATE OR REPLACE
-- cannot drop/reorder a view's output columns (42P16), and this swap
-- removes three, so the view must be dropped and recreated.
DROP VIEW IF EXISTS kb.ontology_terms_current;
CREATE VIEW kb.ontology_terms_current AS
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
    revision.properties
FROM kb.ontology_term_revisions AS revision
ORDER BY revision.term_id, revision.revision DESC, revision.id DESC;

-- Now safe: neither the view nor the trigger function reference these
-- columns any more.
ALTER TABLE kb.ontology_terms
    DROP COLUMN IF EXISTS value_type,
    DROP COLUMN IF EXISTS range_type,
    DROP COLUMN IF EXISTS permitted_unit_term_ids;
ALTER TABLE kb.ontology_term_revisions
    DROP COLUMN IF EXISTS value_type,
    DROP COLUMN IF EXISTS range_type,
    DROP COLUMN IF EXISTS permitted_unit_term_ids;

-- +goose Down
-- Re-add the old columns (and backfill from properties) before anything is
-- made to reference them again, then drop properties last -- same
-- view/function dependency ordering constraint as the Up migration.
ALTER TABLE kb.ontology_terms
    ADD COLUMN IF NOT EXISTS value_type TEXT,
    ADD COLUMN IF NOT EXISTS range_type TEXT,
    ADD COLUMN IF NOT EXISTS permitted_unit_term_ids JSONB;
UPDATE kb.ontology_terms
SET value_type = properties ->> 'value_type',
    range_type = properties ->> 'range_type',
    permitted_unit_term_ids = properties -> 'permitted_unit_term_ids'
WHERE properties IS NOT NULL;

ALTER TABLE kb.ontology_term_revisions
    ADD COLUMN IF NOT EXISTS value_type TEXT,
    ADD COLUMN IF NOT EXISTS range_type TEXT,
    ADD COLUMN IF NOT EXISTS permitted_unit_term_ids JSONB;
UPDATE kb.ontology_term_revisions
SET value_type = properties ->> 'value_type',
    range_type = properties ->> 'range_type',
    permitted_unit_term_ids = properties -> 'permitted_unit_term_ids'
WHERE properties IS NOT NULL;

DROP VIEW IF EXISTS kb.ontology_terms_current;
CREATE VIEW kb.ontology_terms_current AS
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

ALTER TABLE kb.ontology_terms
    DROP COLUMN IF EXISTS properties;
ALTER TABLE kb.ontology_term_revisions
    DROP COLUMN IF EXISTS properties;
