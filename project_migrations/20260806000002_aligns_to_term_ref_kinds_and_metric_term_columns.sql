-- +goose Up
-- Spec 2026080403 §19 step 12 (REQ-2/REQ-3): a keyword concept must be a
-- legal assertion subject for aligns_to_term (subject side, NOT NULL CHECK)
-- and object side (nullable CHECK), and kb.metrics gains the two governed
-- identifiers REQ-3 persists. metric_definition_term_id deliberately has no
-- FK: kb.ontology_terms has no single-column unique constraint on term_id
-- (only (term_id, version)), the same reason every other term-id-reference
-- column in this schema has no DB FK (see §2.4 / AssociateSemantics.termExists).
-- keyword_concept_id *does* get an FK: kb.keyword_concepts.concept_id is a
-- TEXT PRIMARY KEY.

ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT semantic_assertions_subject_ref_kind_check;
ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT semantic_assertions_subject_ref_kind_check
    CHECK (subject_ref_kind IN
           ('object_node', 'ontology_term', 'assertion', 'artifact', 'literal', 'keyword_concept'));

ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT semantic_assertions_object_ref_kind_check;
ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT semantic_assertions_object_ref_kind_check
    CHECK (object_ref_kind IS NULL OR object_ref_kind IN
           ('object_node', 'ontology_term', 'assertion', 'artifact', 'literal', 'keyword_concept'));

ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS keyword_concept_id TEXT
    REFERENCES kb.keyword_concepts(concept_id);
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS metric_definition_term_id TEXT;

CREATE INDEX IF NOT EXISTS idx_kb_semantic_assertions_subject_concept
    ON kb.semantic_assertions (subject_ref_id)
    WHERE subject_ref_kind = 'keyword_concept' AND status = 'accepted';

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_semantic_assertions_subject_concept;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS metric_definition_term_id;
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS keyword_concept_id;
ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT semantic_assertions_object_ref_kind_check;
ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT semantic_assertions_object_ref_kind_check
    CHECK (object_ref_kind IS NULL OR object_ref_kind IN
           ('object_node', 'ontology_term', 'assertion', 'artifact', 'literal'));
ALTER TABLE kb.semantic_assertions
    DROP CONSTRAINT semantic_assertions_subject_ref_kind_check;
ALTER TABLE kb.semantic_assertions
    ADD CONSTRAINT semantic_assertions_subject_ref_kind_check
    CHECK (subject_ref_kind IN
           ('object_node', 'ontology_term', 'assertion', 'artifact', 'literal'));
