-- +goose Up
-- Content rows included in an immutable module release are tagged with the
-- release they shipped in (released_in_release_id) and move to status
-- included_in_release. The linkage makes "which release did this term ship
-- in" a direct query, and lets the module compiler transition exactly the
-- rows it snapshots.
ALTER TABLE kb.ontology_terms         ADD COLUMN IF NOT EXISTS released_in_release_id BIGINT;
ALTER TABLE kb.ontology_term_labels   ADD COLUMN IF NOT EXISTS released_in_release_id BIGINT;
ALTER TABLE kb.ontology_axioms        ADD COLUMN IF NOT EXISTS released_in_release_id BIGINT;
ALTER TABLE kb.ontology_mappings      ADD COLUMN IF NOT EXISTS released_in_release_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_kb_ontology_terms_release
    ON kb.ontology_terms (released_in_release_id);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_term_labels_release
    ON kb.ontology_term_labels (released_in_release_id);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_axioms_release
    ON kb.ontology_axioms (released_in_release_id);
CREATE INDEX IF NOT EXISTS idx_kb_ontology_mappings_release
    ON kb.ontology_mappings (released_in_release_id);

-- +goose Down
ALTER TABLE kb.ontology_terms         DROP COLUMN IF EXISTS released_in_release_id;
ALTER TABLE kb.ontology_term_labels   DROP COLUMN IF EXISTS released_in_release_id;
ALTER TABLE kb.ontology_axioms        DROP COLUMN IF EXISTS released_in_release_id;
ALTER TABLE kb.ontology_mappings      DROP COLUMN IF EXISTS released_in_release_id;
