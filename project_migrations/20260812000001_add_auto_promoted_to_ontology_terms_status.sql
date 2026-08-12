-- +goose Up
-- ADR 2026081201 (auto-promoted governed terms): a metric_definition term
-- can now be created automatically, without curator review, when a
-- keyword_concept has no existing aligns_to_term. 'auto-promoted' marks
-- such a term as live/usable immediately while remaining distinguishable
-- from the human-reviewed 'included_in_release' path for later sampling.
-- Widens the existing CHECK rather than editing the CREATE TABLE migration
-- (db-migration rule: never edit an already-applied migration).
-- +goose StatementBegin
DO $$
DECLARE
    con text;
BEGIN
    SELECT c.conname INTO con
    FROM pg_constraint c
    JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
    WHERE c.conrelid = 'kb.ontology_terms'::regclass
      AND c.contype = 'c'
      AND a.attname = 'status'
    LIMIT 1;
    IF con IS NOT NULL THEN
        EXECUTE format('ALTER TABLE kb.ontology_terms DROP CONSTRAINT %I', con);
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE kb.ontology_terms
    ADD CONSTRAINT ontology_terms_status_check CHECK (status IN (
        'draft', 'in_review', 'approved', 'included_in_release',
        'auto-promoted', 'superseded', 'rejected'
    ));

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE
    con text;
BEGIN
    SELECT c.conname INTO con
    FROM pg_constraint c
    JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
    WHERE c.conrelid = 'kb.ontology_terms'::regclass
      AND c.contype = 'c'
      AND a.attname = 'status'
    LIMIT 1;
    IF con IS NOT NULL THEN
        EXECUTE format('ALTER TABLE kb.ontology_terms DROP CONSTRAINT %I', con);
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE kb.ontology_terms
    ADD CONSTRAINT ontology_terms_status_check CHECK (status IN (
        'draft', 'in_review', 'approved', 'included_in_release',
        'superseded', 'rejected'
    ));
