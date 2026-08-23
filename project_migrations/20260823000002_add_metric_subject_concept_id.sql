-- +goose Up
-- openspec change governed-property-normalization: subject_concept_id gives the
-- metric_subject dimension a keyword-concept identity, resolved the same way
-- keyword_concept_id already is (names.Resolver.ResolveAndObserve), just under
-- a dedicated scope ("metric_subject") so it never collides with metric_name's
-- concept space. Same shape as the existing keyword_concept_id column
-- (project_migrations/20260806000002_aligns_to_term_ref_kinds_and_metric_term_columns.sql).
ALTER TABLE kb.metrics ADD COLUMN IF NOT EXISTS subject_concept_id TEXT
    REFERENCES kb.keyword_concepts(concept_id);

-- +goose Down
ALTER TABLE kb.metrics DROP COLUMN IF EXISTS subject_concept_id;
