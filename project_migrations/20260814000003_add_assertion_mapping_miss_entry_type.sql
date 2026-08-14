-- +goose Up
-- ADR 2026081401 DR2: 'assertion_mapping_miss' is written once per input record per run
-- (by extract_metrics and, as a backstop, associate_semantics) when a kb.metrics row's
-- value_range_type has no approved governed mapping. Family-level entry type name (not
-- metric_-prefixed) since the same free-text-vocabulary shape recurs for other assertion
-- families. Follows the drop/recreate pattern from 20260814000001_add_resolve_metric_entry_type.sql.
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
        'delete_input',
        'resolve_metric',
        'assertion_mapping_miss',
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
        'reconcile_object',
        'delete_input',
        'resolve_metric',
        'pipeline finish'
    ));
