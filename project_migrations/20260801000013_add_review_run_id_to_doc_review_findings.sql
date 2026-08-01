-- +goose Up
-- Ontology-profile findings now carry a real, FK-integrous review run
-- reference instead of relying solely on the pre-existing, client-supplied
-- run_id column (which remains for cross-pass grouping and is unaffected).
ALTER TABLE kb.doc_review_findings
    ADD COLUMN IF NOT EXISTS review_run_id BIGINT REFERENCES kb.ontology_review_runs(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_doc_review_findings_review_run ON kb.doc_review_findings (review_run_id);

-- +goose Down
DROP INDEX IF EXISTS idx_doc_review_findings_review_run;
ALTER TABLE kb.doc_review_findings
    DROP COLUMN IF EXISTS review_run_id;
