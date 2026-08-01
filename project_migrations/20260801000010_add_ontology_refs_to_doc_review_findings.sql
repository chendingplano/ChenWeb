-- +goose Up
-- P4 provenance links for deterministic ontology-profile findings. Existing
-- LLM review findings remain valid and therefore keep these columns nullable.
ALTER TABLE kb.doc_review_findings
    ADD COLUMN IF NOT EXISTS review_scope_id TEXT REFERENCES kb.ontology_review_scopes(review_scope_id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS profile_rule_id BIGINT REFERENCES kb.ontology_profile_rules(id) ON DELETE RESTRICT,
    ADD COLUMN IF NOT EXISTS assertion_id BIGINT REFERENCES kb.semantic_assertions(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS idx_doc_review_findings_scope ON kb.doc_review_findings (review_scope_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_findings_profile_rule ON kb.doc_review_findings (profile_rule_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_findings_assertion ON kb.doc_review_findings (assertion_id);

-- +goose Down
DROP INDEX IF EXISTS idx_doc_review_findings_assertion;
DROP INDEX IF EXISTS idx_doc_review_findings_profile_rule;
DROP INDEX IF EXISTS idx_doc_review_findings_scope;
ALTER TABLE kb.doc_review_findings
    DROP COLUMN IF EXISTS assertion_id,
    DROP COLUMN IF EXISTS profile_rule_id,
    DROP COLUMN IF EXISTS review_scope_id;
