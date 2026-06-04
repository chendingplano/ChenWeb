-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.search_text_join_unique(VARIADIC parts TEXT[])
RETURNS TEXT
LANGUAGE plpgsql
IMMUTABLE
AS $$
DECLARE
    part TEXT;
    normalized TEXT;
    seen TEXT[] := ARRAY[]::TEXT[];
    output_parts TEXT[] := ARRAY[]::TEXT[];
BEGIN
    FOREACH part IN ARRAY parts LOOP
        part := regexp_replace(COALESCE(part, ''), '\s+', ' ', 'g');
        part := btrim(part);
        IF part = '' THEN
            CONTINUE;
        END IF;

        normalized := lower(part);
        IF array_position(seen, normalized) IS NOT NULL THEN
            CONTINUE;
        END IF;

        seen := array_append(seen, normalized);
        output_parts := array_append(output_parts, part);
    END LOOP;

    RETURN array_to_string(output_parts, ' ');
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
    SELECT kb.search_text_join_unique(
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
    );
$$;
-- +goose StatementEnd

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

-- +goose Down
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

DROP FUNCTION IF EXISTS kb.search_text_join_unique(TEXT[]);
