-- +goose Up
-- The spec §9.3 governed ontology-content state machine includes
-- `approved --withdrawn/replaced--> superseded`, so kb.ontology_candidates
-- must accept the superseded status. The original CREATE TABLE CHECK in
-- 20260731000018 was too narrow; this replaces it rather than editing the
-- already-applied migration (db-migration rule).
-- +goose StatementBegin
DO $$
DECLARE
    con text;
BEGIN
    SELECT c.conname INTO con
    FROM pg_constraint c
    JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = ANY(c.conkey)
    WHERE c.conrelid = 'kb.ontology_candidates'::regclass
      AND c.contype = 'c'
      AND a.attname = 'status'
    LIMIT 1;
    IF con IS NOT NULL THEN
        EXECUTE format('ALTER TABLE kb.ontology_candidates DROP CONSTRAINT %I', con);
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE kb.ontology_candidates
    ADD CONSTRAINT ontology_candidates_status_check CHECK (status IN (
        'discovered', 'draft', 'in_review', 'approved', 'included_in_release',
        'rejected', 'deferred', 'superseded'
    ));

-- +goose Down
ALTER TABLE kb.ontology_candidates DROP CONSTRAINT IF EXISTS ontology_candidates_status_check;
