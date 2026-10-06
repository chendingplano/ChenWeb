-- +goose Up
-- Independently reviewed benchmarks; never populated by extract_metrics itself.
CREATE SCHEMA IF NOT EXISTS testbed;
CREATE TABLE IF NOT EXISTS testbed.metrics (
    id BIGSERIAL PRIMARY KEY,
    input_record_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    benchmark_run_id UUID NOT NULL,
    metric_id TEXT NOT NULL,
    source_filename TEXT NOT NULL,
    source_sha256 TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    metric_name TEXT NOT NULL,
    metric_name_en TEXT,
    metric_subject TEXT NOT NULL,
    metric_subject_en TEXT,
    metric_desc TEXT NOT NULL,
    metric_desc_en TEXT,
    metric_context TEXT NOT NULL,
    metric_context_en TEXT,
    metric_keywords JSONB NOT NULL,
    metric_keywords_en JSONB,
    source_line_spans JSONB NOT NULL,
    source_table_rows JSONB NOT NULL DEFAULT '[]'::jsonb,
    location_type TEXT NOT NULL CHECK (location_type IN
        ('sentence', 'bullet', 'table_row', 'table_cell', 'heading_context', 'mixed')),
    metric_unit TEXT NOT NULL,
    metric_unit_en TEXT,
    metric_value TEXT NOT NULL,
    value_data_type TEXT NOT NULL,
    value_range_type TEXT NOT NULL CHECK (value_range_type IN
        ('lower_bound', 'upper_bound', 'exact', 'range', 'qualitative', 'limit_absent')),
    value_class TEXT NOT NULL CHECK (value_class IN
        ('observation', 'requirement', 'target', 'reference', 'design_capability', 'definition')),
    value_class_en TEXT,
    value_min DOUBLE PRECISION,
    value_max DOUBLE PRECISION,
    condition TEXT NOT NULL DEFAULT '',
    formula_or_definition TEXT NOT NULL,
    threshold_or_target TEXT NOT NULL,
    measurement_frequency TEXT NOT NULL,
    metric_categories JSONB NOT NULL,
    confidence DOUBLE PRECISION NOT NULL CHECK (confidence BETWEEN 0 AND 1),
    is_explicit_metric BOOLEAN NOT NULL,
    table_name_or_section TEXT NOT NULL,
    reasoning_tags JSONB NOT NULL,
    ext_info JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (input_record_id, benchmark_run_id, metric_id),
    CHECK (jsonb_typeof(source_line_spans) = 'array' AND jsonb_array_length(source_line_spans) > 0),
    CHECK (jsonb_typeof(metric_categories) = 'array' AND jsonb_array_length(metric_categories) > 0)
);
CREATE INDEX IF NOT EXISTS idx_testbed_metrics_document_run
    ON testbed.metrics (input_record_id, benchmark_run_id);

-- +goose Down
DROP TABLE IF EXISTS testbed.metrics;
-- testbed is shared by other test tools; retain the schema.
