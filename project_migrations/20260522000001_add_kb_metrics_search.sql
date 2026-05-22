-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.metric_search_jsonb_array_text(arr JSONB)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT CASE jsonb_typeof(COALESCE(arr, 'null'::jsonb))
        WHEN 'array' THEN (
            SELECT COALESCE(string_agg(value, ' '), '')
            FROM jsonb_array_elements_text(arr) AS t(value)
        )
        WHEN 'string' THEN trim(both '"' from arr::text)
        WHEN 'number' THEN arr::text
        WHEN 'boolean' THEN arr::text
        ELSE ''
    END;
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.metric_search_document(
    metric_name TEXT,
    metric_name_en TEXT,
    metric_subject TEXT,
    metric_subject_en TEXT,
    metric_keywords JSONB,
    metric_keywords_en JSONB,
    metric_desc TEXT,
    metric_desc_en TEXT,
    metric_context TEXT,
    metric_context_en TEXT,
    value_class TEXT,
    value_class_en TEXT,
    metric_unit TEXT,
    metric_unit_en TEXT,
    table_name_or_section TEXT,
    category_paths JSONB,
    category_paths_en JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(
        concat_ws(
            ' ',
            COALESCE(metric_name, ''),
            COALESCE(metric_name_en, ''),
            COALESCE(metric_subject, ''),
            COALESCE(metric_subject_en, ''),
            kb.metric_search_jsonb_array_text(metric_keywords),
            kb.metric_search_jsonb_array_text(metric_keywords_en),
            COALESCE(metric_desc, ''),
            COALESCE(metric_desc_en, ''),
            COALESCE(metric_context, ''),
            COALESCE(metric_context_en, ''),
            COALESCE(value_class, ''),
            COALESCE(value_class_en, ''),
            COALESCE(metric_unit, ''),
            COALESCE(metric_unit_en, ''),
            COALESCE(table_name_or_section, ''),
            kb.metric_search_jsonb_array_text(category_paths),
            kb.metric_search_jsonb_array_text(category_paths_en)
        )
    );
$$;
-- +goose StatementEnd

ALTER TABLE kb.metrics
    ADD COLUMN IF NOT EXISTS search_document TEXT,
    ADD COLUMN IF NOT EXISTS search_vector TSVECTOR;

UPDATE kb.metrics
SET
    search_document = kb.metric_search_document(
        metric_name, metric_name_en,
        metric_subject, metric_subject_en,
        metric_keywords, metric_keywords_en,
        metric_desc, metric_desc_en,
        metric_context, metric_context_en,
        value_class, value_class_en,
        metric_unit, metric_unit_en,
        table_name_or_section,
        category_paths, category_paths_en
    ),
    search_vector = to_tsvector(
        'simple',
        kb.metric_search_document(
            metric_name, metric_name_en,
            metric_subject, metric_subject_en,
            metric_keywords, metric_keywords_en,
            metric_desc, metric_desc_en,
            metric_context, metric_context_en,
            value_class, value_class_en,
            metric_unit, metric_unit_en,
            table_name_or_section,
            category_paths, category_paths_en
        )
    );

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.refresh_metric_search_columns()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.search_document := kb.metric_search_document(
        NEW.metric_name, NEW.metric_name_en,
        NEW.metric_subject, NEW.metric_subject_en,
        NEW.metric_keywords, NEW.metric_keywords_en,
        NEW.metric_desc, NEW.metric_desc_en,
        NEW.metric_context, NEW.metric_context_en,
        NEW.value_class, NEW.value_class_en,
        NEW.metric_unit, NEW.metric_unit_en,
        NEW.table_name_or_section,
        NEW.category_paths, NEW.category_paths_en
    );
    NEW.search_vector := to_tsvector('simple', COALESCE(NEW.search_document, ''));
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_refresh_metric_search_columns ON kb.metrics;
CREATE TRIGGER trg_refresh_metric_search_columns
BEFORE INSERT OR UPDATE OF
    metric_name, metric_name_en,
    metric_subject, metric_subject_en,
    metric_keywords, metric_keywords_en,
    metric_desc, metric_desc_en,
    metric_context, metric_context_en,
    value_class, value_class_en,
    metric_unit, metric_unit_en,
    table_name_or_section,
    category_paths, category_paths_en
ON kb.metrics
FOR EACH ROW
EXECUTE FUNCTION kb.refresh_metric_search_columns();

CREATE INDEX IF NOT EXISTS idx_kb_metrics_search_vector
ON kb.metrics
USING GIN (search_vector);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_metrics_search_vector;
DROP TRIGGER IF EXISTS trg_refresh_metric_search_columns ON kb.metrics;
DROP FUNCTION IF EXISTS kb.refresh_metric_search_columns();
ALTER TABLE kb.metrics
    DROP COLUMN IF EXISTS search_vector,
    DROP COLUMN IF EXISTS search_document;
DROP FUNCTION IF EXISTS kb.metric_search_document(TEXT, TEXT, TEXT, TEXT, JSONB, JSONB, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, JSONB, JSONB);
DROP FUNCTION IF EXISTS kb.metric_search_jsonb_array_text(JSONB);
