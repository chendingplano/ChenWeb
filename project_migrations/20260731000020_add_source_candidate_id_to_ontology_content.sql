-- +goose Up
-- Content rows materialized from an approved candidate carry
-- source_candidate_id so promotion is idempotent (a retry after a transient
-- failure does not duplicate the row; spec §16.3 item 13) and provenance is
-- traceable back to the proposing candidate. kb.ontology_terms already has
-- the column from its CREATE TABLE; labels/axioms/mappings get it here.
ALTER TABLE kb.ontology_term_labels ADD COLUMN IF NOT EXISTS source_candidate_id BIGINT;
ALTER TABLE kb.ontology_axioms     ADD COLUMN IF NOT EXISTS source_candidate_id BIGINT;
ALTER TABLE kb.ontology_mappings   ADD COLUMN IF NOT EXISTS source_candidate_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_kb_ontology_term_labels_source_candidate
    ON kb.ontology_term_labels (source_candidate_id);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_axioms_source_candidate
    ON kb.ontology_axioms (source_candidate_id);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_mappings_source_candidate
    ON kb.ontology_mappings (source_candidate_id);

-- +goose Down
ALTER TABLE kb.ontology_term_labels DROP COLUMN IF EXISTS source_candidate_id;
ALTER TABLE kb.ontology_axioms     DROP COLUMN IF EXISTS source_candidate_id;
ALTER TABLE kb.ontology_mappings   DROP COLUMN IF EXISTS source_candidate_id;
