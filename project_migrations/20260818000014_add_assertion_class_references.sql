-- +goose Up
-- ADR 2026081701 DR1/DR2: assertions point to the stable class identity and
-- may retain the immutable contract revision used during normalization. The
-- columns are additive; lifecycle and represented-source admission remain
-- independent from class resolution.

ALTER TABLE kb.semantic_assertions
    ADD COLUMN IF NOT EXISTS instance_of_term_id TEXT;

ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT fk_kb_semantic_assertions_instance_of_term
    FOREIGN KEY (instance_of_term_id)
    REFERENCES kb.ontology_term_headers (term_id)
    NOT VALID;

ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT fk_kb_semantic_assertions_normalized_contract_revision
    FOREIGN KEY (normalized_against_contract_revision_id)
    REFERENCES kb.ontology_class_contract_revisions (id)
    NOT VALID;

-- Relation-only assertions retain a NULL class reference. Once an assertion
-- declares a resolved/provisional/ambiguous class state, it must carry the
-- stable term ID rather than a mutable contract identity.
ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT ck_kb_semantic_assertions_resolved_class_reference
    CHECK (
        class_identity_state_term_id NOT IN (
            'semantic:resolved_existing',
            'semantic:provisional_new',
            'semantic:ambiguous_candidates'
        )
        OR instance_of_term_id IS NOT NULL
    ) NOT VALID;

CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_instance_of_term
    ON kb.semantic_assertions (instance_of_term_id)
    WHERE instance_of_term_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_normalized_contract_revision
    ON kb.semantic_assertions (normalized_against_contract_revision_id)
    WHERE normalized_against_contract_revision_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_semantic_assertions_normalized_contract_revision;
DROP INDEX IF EXISTS kb.idx_kb_semantic_assertions_instance_of_term;
ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT IF EXISTS ck_kb_semantic_assertions_resolved_class_reference,
    DROP CONSTRAINT IF EXISTS fk_kb_semantic_assertions_normalized_contract_revision,
    DROP CONSTRAINT IF EXISTS fk_kb_semantic_assertions_instance_of_term,
    DROP COLUMN IF EXISTS instance_of_term_id;
