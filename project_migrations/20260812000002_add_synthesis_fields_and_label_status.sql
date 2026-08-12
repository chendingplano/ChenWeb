-- +goose Up
-- ADR 2026081201 (auto-promoted governed terms), DR3: an auto-created
-- metric_definition term's payload is synthesized from the triggering
-- metric's own extracted structured fields. kb.ontology_terms had no
-- columns for value_type/range_type/permitted units -- DR23 (ADR
-- 2026072901 §3.24) described them as part of a metric_definition term's
-- content, but promoteTerm (candidates/promote.go) never actually
-- persisted them; only definition/scope survive promotion today. This
-- migration adds the missing columns so this change has somewhere real to
-- write them, for both the auto-promoted path and (going forward) the
-- reviewed candidate path.
ALTER TABLE kb.ontology_terms
    ADD COLUMN IF NOT EXISTS value_type TEXT,
    ADD COLUMN IF NOT EXISTS range_type TEXT,
    ADD COLUMN IF NOT EXISTS permitted_unit_term_ids JSONB;

-- Also widen kb.ontology_term_labels' status CHECK to accept 'auto-promoted',
-- so a label created alongside an auto-promoted term can carry the same
-- status rather than being forced into a misleading reviewed-sounding value.
-- +goose StatementBegin
DO $$
DECLARE
    con text;
BEGIN
    SELECT c.conname INTO con
    FROM pg_constraint c
    JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
    WHERE c.conrelid = 'kb.ontology_term_labels'::regclass
      AND c.contype = 'c'
      AND a.attname = 'status'
    LIMIT 1;
    IF con IS NOT NULL THEN
        EXECUTE format('ALTER TABLE kb.ontology_term_labels DROP CONSTRAINT %I', con);
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE kb.ontology_term_labels
    ADD CONSTRAINT ontology_term_labels_status_check CHECK (status IN (
        'draft', 'in_review', 'approved', 'included_in_release',
        'auto-promoted', 'superseded', 'rejected'
    ));

-- +goose Down
ALTER TABLE kb.ontology_terms
    DROP COLUMN IF EXISTS value_type,
    DROP COLUMN IF EXISTS range_type,
    DROP COLUMN IF EXISTS permitted_unit_term_ids;

-- +goose StatementBegin
DO $$
DECLARE
    con text;
BEGIN
    SELECT c.conname INTO con
    FROM pg_constraint c
    JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
    WHERE c.conrelid = 'kb.ontology_term_labels'::regclass
      AND c.contype = 'c'
      AND a.attname = 'status'
    LIMIT 1;
    IF con IS NOT NULL THEN
        EXECUTE format('ALTER TABLE kb.ontology_term_labels DROP CONSTRAINT %I', con);
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE kb.ontology_term_labels
    ADD CONSTRAINT ontology_term_labels_status_check CHECK (status IN (
        'draft', 'in_review', 'approved', 'included_in_release',
        'superseded', 'rejected'
    ));
