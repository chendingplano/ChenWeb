-- +goose Up
-- ADR 2026061302: Phase 1/Phase 2 split logs separate entry types
-- ('extract_entities', 'extract_relations'). Add them to the doc_proc_logs
-- check constraint. 'extract_entity_relation' is retained so the ADD CONSTRAINT
-- still validates pre-existing rows written under the old combined entry type.
ALTER TABLE IF EXISTS kb.doc_proc_logs
    DROP CONSTRAINT IF EXISTS doc_proc_logs_entry_type_check;

ALTER TABLE IF EXISTS kb.doc_proc_logs
    ADD CONSTRAINT doc_proc_logs_entry_type_check
    CHECK (entry_type IN (
        'llm_call',
        'doc_proc_summary',
        'generate_summary',
        'generate_summary_finish',
        'extract_topics',
        'extract_topics_finish',
        'static_analyzer',
        'blocking',
        'extract_metrics',
        'extract_metrics_finish',
        'extract_projections',
        'enrich_projections',
        'extract_projections_finish',
        'extract_provisions',
        'extract_provisions_finish',
        'extract_scene_blocks',
        'enrich_scene_blocks',
        'extract_scene_blocks_finish',
        'extract_structured_knowledge',
        'enrich_structured_knowledge',
        'extract_entity_relation',
        'extract_entities',
        'extract_relations',
        'extract_entity_relation_finish',
        'enrich_metrics',
        'extract_inventory_items',
        'extract_inventory_items_finish',
        'extract_doc_metadata',
        'chunking',
        'generate_topics',
        'pipeline finish'
    ));

-- +goose Down
ALTER TABLE IF EXISTS kb.doc_proc_logs
    DROP CONSTRAINT IF EXISTS doc_proc_logs_entry_type_check;

ALTER TABLE IF EXISTS kb.doc_proc_logs
    ADD CONSTRAINT doc_proc_logs_entry_type_check
    CHECK (entry_type IN (
        'llm_call',
        'doc_proc_summary',
        'generate_summary',
        'generate_summary_finish',
        'extract_topics',
        'extract_topics_finish',
        'static_analyzer',
        'blocking',
        'extract_metrics',
        'extract_metrics_finish',
        'extract_projections',
        'enrich_projections',
        'extract_projections_finish',
        'extract_provisions',
        'extract_provisions_finish',
        'extract_scene_blocks',
        'enrich_scene_blocks',
        'extract_scene_blocks_finish',
        'extract_structured_knowledge',
        'enrich_structured_knowledge',
        'extract_entity_relation',
        'extract_entity_relation_finish',
        'enrich_metrics',
        'extract_inventory_items',
        'extract_inventory_items_finish',
        'extract_doc_metadata',
        'chunking',
        'generate_topics',
        'pipeline finish'
    ));
