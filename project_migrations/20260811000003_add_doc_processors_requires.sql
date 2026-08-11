-- +goose Up
-- Dependency list: name_as_id values of the doc processors this processor
-- depends on (the seeded notes encode the same dependencies as prose).
ALTER TABLE kb.doc_processors
    ADD COLUMN IF NOT EXISTS requires JSONB NOT NULL DEFAULT '[]'::jsonb;

-- Backfill the seeded roster's documented dependencies (2026-08-11 seed) as
-- name_as_id references. Optional/ambiguous dependencies keep the documented
-- default; processors with no named dependency stay [].
UPDATE kb.doc_processors SET requires = '["structure_analyzer"]'::jsonb WHERE name_as_id = 'chunking';
UPDATE kb.doc_processors SET requires = '["blocking"]'::jsonb WHERE name_as_id = 'extract_metadata';
UPDATE kb.doc_processors SET requires = '["chunking"]'::jsonb WHERE name_as_id IN (
    'extract_metrics', 'extract_provisions', 'extract_semantic_projections',
    'generate_summaries', 'generate_topics', 'generate_scene_blocks',
    'extract_entity_relation', 'extract_inventory_items', 'review_document',
    'extract_metric_definitions', 'extract_test_methods', 'classify_document'
);
UPDATE kb.doc_processors SET requires = '["extract_metrics","extract_provisions"]'::jsonb WHERE name_as_id = 'normalize_assertions';
UPDATE kb.doc_processors SET requires = '["normalize_assertions"]'::jsonb WHERE name_as_id = 'associate_semantics';
UPDATE kb.doc_processors SET requires = '["associate_semantics"]'::jsonb WHERE name_as_id = 'project_semantics';
UPDATE kb.doc_processors SET requires = '["extract_metadata"]'::jsonb WHERE name_as_id = 'facet_tier2';

-- +goose Down
ALTER TABLE kb.doc_processors
    DROP COLUMN IF EXISTS requires;
