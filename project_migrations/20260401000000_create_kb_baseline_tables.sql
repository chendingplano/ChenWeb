-- +goose Up
-- Baseline creation for kb.inputs, kb.metrics, kb.semantic_projections.
--
-- These three tables predate this migration directory (they existed on the
-- long-running dev database before migration tracking began) and were never
-- captured by a CREATE TABLE anywhere here, even though dozens of later
-- migrations ALTER them and the earliest of those (20260420113000_create_kb_
-- chunks_table.sql) already REFERENCES kb.inputs(id). On a truly fresh
-- database this made every such migration fail with
-- "relation kb.inputs does not exist" — see devdoc
-- KnowledgeStore/doc-repo/devdocs/202607/2026072401-devdoc-deploy-production.md §4/§9.
--
-- Column sets below are the live schema as of 2026-07-25 MINUS every column a
-- later migration in this directory still adds itself (verified by grepping
-- every ADD COLUMN target for these three tables across this whole
-- directory) — so this file only supplies what genuinely predates all
-- migration history, and every later ADD/DROP COLUMN IF [NOT] EXISTS remains
-- a correct, idempotent no-op-or-apply exactly as it already behaves against
-- the real dev database. Two exceptions handled deliberately:
--   - kb.metrics.line_range / kb.semantic_projections.line_range are
--     GENERATED columns whose function (kb.line_spans_to_int8multirange) is
--     only created by 20260616000001_add_line_range_to_kb_metrics.sql /
--     20260616000002_add_line_range_to_artifact_tables.sql — excluded here,
--     those migrations ADD COLUMN IF NOT EXISTS them once the function exists.
--   - kb.metrics.event_id was originally named extract_id and NOT NULL;
--     20260507100001_rename_kb_metrics_extract_id_to_event_id.sql renames it
--     and drops the NOT NULL. This baseline creates the column directly as
--     the final `event_id` (nullable), so that rename migration was patched
--     (2026-07-25) to skip itself when extract_id doesn't exist.
--
-- Verified against the live database (2026-07-25) column-by-column and
-- constraint-by-constraint, not just by successfully running `goose up` from
-- an empty database — two things a naive "final shape" dump would have
-- gotten wrong:
--   - kb.inputs.md5 is VARCHAR(64) here, not the TEXT that
--     20260609000001_add_kb_inputs_md5.sql's ADD COLUMN specifies — md5 is
--     itself another phantom pre-existing column, so that migration's ADD
--     COLUMN IF NOT EXISTS has always been a silent no-op against the real
--     dev database, and its stated type never actually took effect.
--   - kb.metrics.input_record_id / kb.semantic_projections.input_record_id
--     have NO foreign key to kb.inputs on the live database (unlike e.g.
--     kb.provisions/kb.chunks, which were created by real migrations with
--     the FK from day one) — do not add one here, it would be a new
--     constraint that doesn't reflect reality and could reject rows that
--     are valid today.
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.inputs (
    id BIGSERIAL PRIMARY KEY,
    staging_filename TEXT,
    type VARCHAR(50) NOT NULL,
    title TEXT,
    doc_no VARCHAR(255),
    source TEXT,
    file_name TEXT,
    backup_filename TEXT,
    result_filename TEXT,
    publish_date DATE,
    authors TEXT,
    owner BIGINT,
    status JSONB NOT NULL DEFAULT '[]'::jsonb,
    create_time TIMESTAMPTZ NOT NULL DEFAULT now(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT now(),
    public_info JSONB,
    private_info JSONB,
    notes TEXT,
    error_msg TEXT,
    md5 VARCHAR(64),
    parser_name VARCHAR(64),
    doc_metadata JSONB
);

CREATE INDEX IF NOT EXISTS idx_kb_inputs_create_time ON kb.inputs (create_time DESC);
CREATE INDEX IF NOT EXISTS idx_kb_inputs_owner ON kb.inputs (owner);
CREATE INDEX IF NOT EXISTS idx_kb_inputs_type ON kb.inputs (type);

CREATE TABLE IF NOT EXISTS kb.metrics (
    id BIGSERIAL PRIMARY KEY,
    input_record_id BIGINT NOT NULL,
    event_id TEXT,
    metric_name TEXT,
    source_line_spans JSONB,
    metric_subject TEXT,
    metric_desc TEXT,
    metric_context TEXT,
    metric_keywords JSONB,
    location_type TEXT,
    metric_unit TEXT,
    formula_or_definition TEXT,
    threshold_or_target TEXT,
    measurement_frequency TEXT,
    confidence DOUBLE PRECISION,
    is_explicit_metric BOOLEAN,
    reasoning_tags JSONB,
    ext_info JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS kb.semantic_projections (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    semantic_proj_id TEXT,
    language TEXT,
    descriptive_name TEXT,
    descriptive_name_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    category_paths JSONB,
    category_paths_en JSONB,
    model_name TEXT,
    prompt_name TEXT,
    search_document TEXT,
    search_vector TSVECTOR,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS kb.semantic_projections;
DROP TABLE IF EXISTS kb.metrics;
DROP TABLE IF EXISTS kb.inputs;
