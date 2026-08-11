-- +goose Up
-- doc_facet_values_vocabulary_release_id_fkey was added directly to the
-- database out-of-band (never via a goose migration -- confirmed absent
-- from every migration file and from project_db_migration's applied
-- history). It is incompatible with 20260801000016_create_kb_doc_facet_values.sql,
-- which declares vocabulary_release_id as a plain BIGINT NOT NULL DEFAULT 0
-- with no FK: tier-1/tier-2 facet writers (facet_tier1.go/facet_tier2.go)
-- intentionally leave VocabularyReleaseID at its Go zero value for facets
-- that aren't release-scoped, and kb.ontology_module_releases has no id=0
-- row, so every such insert was failing with a 23503 foreign key violation.
-- This migration removes the stray constraint to restore parity with the
-- originally committed schema.
ALTER TABLE kb.doc_facet_values
    DROP CONSTRAINT IF EXISTS doc_facet_values_vocabulary_release_id_fkey;

-- +goose Down
ALTER TABLE kb.doc_facet_values
    ADD CONSTRAINT doc_facet_values_vocabulary_release_id_fkey
        FOREIGN KEY (vocabulary_release_id) REFERENCES kb.ontology_module_releases(id) ON DELETE RESTRICT;
