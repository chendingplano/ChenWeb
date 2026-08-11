-- +goose Up
-- Admin catalog of doc processors (capsule §7 "Doc Processing Pipeline"). This is an
-- editorial/management table — the pipeline execution machinery does not read it at runtime;
-- execution metadata lives in kb.processor_registry and the Go literal productionProcessorSpecs.
CREATE TABLE IF NOT EXISTS kb.doc_processors (
    name_as_id   VARCHAR(128) PRIMARY KEY,   -- canonical processor name (capsule §7)
    display_name VARCHAR(255) NOT NULL,
    description  TEXT,
    type         VARCHAR(16) NOT NULL CHECK (type IN ('mandatory', 'configurable')),
    require_llm  BOOLEAN NOT NULL DEFAULT false,
    status       VARCHAR(16) NOT NULL DEFAULT 'active'
                 CHECK (status IN ('active', 'disabled', 'suspended')),
    notes        TEXT,
    create_time  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Initial roster seeded from capsule §7 (2026-08-11). Routed / routed (Phase C) processors are
-- typed 'configurable' here (user decision): their routing behavior lives in kb.processor_registry
-- and kb.pipeline_rules, and the §7 mandatory/configurable axis is what this catalog manages.
-- Idempotent: never clobbers an admin edit or a later manual insert.
INSERT INTO kb.doc_processors
    (name_as_id, display_name, description, type, require_llm, status, notes)
VALUES
    ('blocking', 'Blocking Processor',
     'Breaks the input file into blocks; always executed. Output feeds the block buffer.',
     'mandatory', false, 'active', 'seqno 1 · depends on: none (input file buffer)'),
    ('structure_analyzer', 'Structure Analyzer',
     'Doc structure static analyzer; always executed, runs before chunking.',
     'mandatory', false, 'active', 'seqno 2 · depends on: none'),
    ('chunking', 'Chunking Processor',
     'Breaks blocks into chunks; always executed, depends on structure analysis output.',
     'mandatory', false, 'active', 'seqno 3 · depends on: 2 (structure_analyzer)'),
    ('extract_metadata', 'Extract Doc Metadata',
     'Extracts document metadata; always executed, uses the blocking output.',
     'mandatory', true, 'active', 'seqno 4 · depends on: 1 (blocking)'),
    ('extract_metrics', 'Extract Metrics',
     'Extracts metrics from chunks; configurable.',
     'configurable', true, 'active', 'seqno 5 · depends on: 3 (chunking)'),
    ('extract_provisions', 'Extract Provisions',
     'Extracts provisions; depends on chunking by default, or blocking when EXTRACT_PROVISIONS_INPUT=blocks.',
     'configurable', true, 'active', 'seqno 6 · depends on: 3 (chunking) or 1 (blocking)'),
    ('extract_semantic_projections', 'Extract Semantic Projections',
     'Extracts semantic projections from chunks; configurable.',
     'configurable', true, 'active', 'seqno 7 · depends on: 3 (chunking)'),
    ('generate_summaries', 'Generate Summaries',
     'Generates summaries from chunks; configurable.',
     'configurable', true, 'active', 'seqno 8 · depends on: 3 (chunking)'),
    ('generate_topics', 'Generate Topics',
     'Generates topics from chunks; configurable.',
     'configurable', true, 'active', 'seqno 9 · depends on: 3 (chunking)'),
    ('generate_scene_blocks', 'Generate Scene Blocks',
     'Generates scene blocks from chunks; configurable.',
     'configurable', true, 'active', 'seqno 10 · depends on: 3 (chunking)'),
    ('extract_entity_relation', 'Extract Entities & Relations',
     'Extracts entities and relations from chunks; configurable.',
     'configurable', true, 'active', 'seqno 11 · depends on: 3 (chunking)'),
    ('extract_inventory_items', 'Extract Inventory Items',
     'Extracts inventory item objects from chunks; configurable.',
     'configurable', true, 'active', 'seqno 12 · depends on: 3 (chunking)'),
    ('review_document', 'Document Review',
     'Document review: LLM-powered multi-aspect review pipeline; on-demand only.',
     'configurable', true, 'active', 'seqno 13 · depends on: 3 (chunking) · ADR 2026061801'),
    ('extract_metric_definitions', 'Extract Metric Definitions',
     'Proposes governed metric_definition candidates from explicit definitions; metric values alone are excluded. Routed.',
     'configurable', true, 'active', 'seqno 14 · depends on: 3 (chunking) · routed'),
    ('extract_test_methods', 'Extract Test Methods',
     'Proposes procedure-term and explicit metric-to-procedure (mea:measured_by) candidates with source spans. Routed.',
     'configurable', true, 'active', 'seqno 15 · depends on: 3 (chunking) · routed'),
    ('extract_product_structure', 'Extract Product Structure',
     'Converts only explicit part_of/component_of relations with reconciled object endpoints into structural decision candidates. Routed post-process; no additional LLM.',
     'configurable', false, 'active', 'seqno 16 · depends on: entity/relation post-process · routed'),
    ('normalize_assertions', 'Normalize Assertions',
     'DR8 Phase D stage 1: turns each artifact family output into candidate qualified assertions. Inert unless SEMANTIC_ASSOCIATION_ENABLED. Routed.',
     'configurable', false, 'active', 'seqno 17 · depends on: 5, 6 · routed (Phase C)'),
    ('associate_semantics', 'Associate Semantics',
     'DR8 Phase D stage 2: resolve, validate, adjudicate, and persist stage-1 candidates as accepted assertions. Inert unless SEMANTIC_ASSOCIATION_ENABLED. Routed.',
     'configurable', false, 'active', 'seqno 18 · depends on: 17 · routed (Phase C)'),
    ('project_semantics', 'Project Semantics',
     'DR8 Phase D stage 3: builds derived projections from accepted assertions and logs the association-run report. Inert unless SEMANTIC_ASSOCIATION_ENABLED. Routed.',
     'configurable', false, 'active', 'seqno 19 · depends on: 18 · routed (Phase C)'),
    ('classify_document', 'Classify Document',
     'Tier-3 governed-vocabulary classifier (document.doc_kind/domain/normative_status/jurisdiction); resolver-invoked, not wave-dispatched. Routed.',
     'configurable', true, 'active', 'seqno 20 · depends on: 3 (chunking) · routed'),
    ('facet_tier1', 'Facet Tier 1',
     'Tier-1 deterministic document-facet producer; runs right after the line file is parsed, before Phase A; registry-only, non-wave-dispatched. Routed.',
     'configurable', false, 'active', 'seqno 21 · depends on: none · routed'),
    ('facet_tier2', 'Facet Tier 2',
     'Tier-2 document-facet producer; runs inside extract_metadata HandleEvent after it persists doc_no/publish_date; registry-only, non-wave-dispatched. Routed.',
     'configurable', false, 'active', 'seqno 22 · depends on: 4 (extract_metadata) · routed')
ON CONFLICT (name_as_id) DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS kb.doc_processors;
