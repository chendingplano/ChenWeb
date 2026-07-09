-- +goose Up
-- Re-apply the reconcile_object doc_proc_logs entry type with a fresh version.
-- Some staging DBs recorded the former duplicate version 20260708000001 for a
-- different migration, causing 20260708000001_add_reconcile_object_entry_type.sql
-- to be skipped even after the filename collision was fixed.
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
        'reconcile_object',
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
