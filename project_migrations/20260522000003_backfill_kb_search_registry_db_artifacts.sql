-- +goose Up
INSERT INTO kb.search_artifacts (
    artifact_type,
    artifact_id,
    input_record_id,
    source_row_id,
    primary_label,
    secondary_label,
    search_document,
    search_vector,
    snippet_basis,
    source_title,
    source_filename,
    category_paths,
    source_line_spans,
    semantic_payload,
    updated_at
)
SELECT
    'metric' AS artifact_type,
    m.input_record_id::text || '_mtc_' || COALESCE(NULLIF(regexp_replace(COALESCE(m.metric_id, ''), '^.*_', ''), ''), m.id::text) AS artifact_id,
    m.input_record_id,
    m.id AS source_row_id,
    COALESCE(NULLIF(m.metric_name, ''), NULLIF(m.metric_subject, ''), 'Metric #' || m.id::text) AS primary_label,
    COALESCE(NULLIF(m.metric_unit, ''), NULLIF(m.metric_unit_en, ''), '') AS secondary_label,
    COALESCE(m.search_document, '') AS search_document,
    COALESCE(m.search_vector, to_tsvector('simple', COALESCE(m.search_document, ''))) AS search_vector,
    COALESCE(NULLIF(m.metric_desc, ''), NULLIF(m.metric_context, ''), NULLIF(m.metric_name, ''), '') AS snippet_basis,
    COALESCE(i.file_name, '') AS source_title,
    COALESCE(i.file_name, '') AS source_filename,
    COALESCE(m.category_paths, '[]'::jsonb) AS category_paths,
    COALESCE(m.source_line_spans, '[]'::jsonb) AS source_line_spans,
    jsonb_build_object(
        'metric_unit', COALESCE(m.metric_unit, ''),
        'metric_desc', COALESCE(m.metric_desc, ''),
        'metric_keywords', COALESCE(m.metric_keywords, '[]'::jsonb)
    ) AS semantic_payload,
    NOW() AS updated_at
FROM kb.metrics m
JOIN kb.inputs i ON i.id = m.input_record_id
ON CONFLICT (artifact_type, artifact_id) DO UPDATE SET
    input_record_id = EXCLUDED.input_record_id,
    source_row_id = EXCLUDED.source_row_id,
    primary_label = EXCLUDED.primary_label,
    secondary_label = EXCLUDED.secondary_label,
    search_document = EXCLUDED.search_document,
    search_vector = EXCLUDED.search_vector,
    snippet_basis = EXCLUDED.snippet_basis,
    source_title = EXCLUDED.source_title,
    source_filename = EXCLUDED.source_filename,
    category_paths = EXCLUDED.category_paths,
    source_line_spans = EXCLUDED.source_line_spans,
    semantic_payload = EXCLUDED.semantic_payload,
    updated_at = NOW();

INSERT INTO kb.search_artifacts (
    artifact_type,
    artifact_id,
    input_record_id,
    source_row_id,
    primary_label,
    secondary_label,
    search_document,
    search_vector,
    snippet_basis,
    source_title,
    source_filename,
    category_paths,
    source_line_spans,
    semantic_payload,
    updated_at
)
SELECT
    'provision' AS artifact_type,
    p.input_record_id::text || '_prv_' || COALESCE(NULLIF(trim(COALESCE(p.prov_id::text, '')), ''), p.id::text) AS artifact_id,
    p.input_record_id,
    p.id AS source_row_id,
    COALESCE(NULLIF(p.prov_name, ''), COALESCE(NULLIF(p.provision_subject, ''), 'Provision #' || p.id::text)) AS primary_label,
    COALESCE(NULLIF(p.provision_type, ''), '') AS secondary_label,
    COALESCE(p.search_document, '') AS search_document,
    COALESCE(p.search_vector, to_tsvector('simple', COALESCE(p.search_document, ''))) AS search_vector,
    COALESCE(NULLIF(p.prov_desc, ''), NULLIF(p.source_text, ''), NULLIF(p.prov_name, ''), '') AS snippet_basis,
    COALESCE(p.input_filename, '') AS source_title,
    COALESCE(p.input_filename, '') AS source_filename,
    COALESCE(p.category_paths, '[]'::jsonb) AS category_paths,
    COALESCE(p.source_line_spans, '[]'::jsonb) AS source_line_spans,
    jsonb_build_object(
        'provision_type', COALESCE(p.provision_type, ''),
        'prov_desc', COALESCE(p.prov_desc, ''),
        'keywords', COALESCE(p.provision_keywords, '[]'::jsonb)
    ) AS semantic_payload,
    NOW() AS updated_at
FROM kb.provisions p
WHERE EXISTS (
    SELECT 1
    FROM kb.inputs i
    WHERE i.id = p.input_record_id
)
ON CONFLICT (artifact_type, artifact_id) DO UPDATE SET
    input_record_id = EXCLUDED.input_record_id,
    source_row_id = EXCLUDED.source_row_id,
    primary_label = EXCLUDED.primary_label,
    secondary_label = EXCLUDED.secondary_label,
    search_document = EXCLUDED.search_document,
    search_vector = EXCLUDED.search_vector,
    snippet_basis = EXCLUDED.snippet_basis,
    source_title = EXCLUDED.source_title,
    source_filename = EXCLUDED.source_filename,
    category_paths = EXCLUDED.category_paths,
    source_line_spans = EXCLUDED.source_line_spans,
    semantic_payload = EXCLUDED.semantic_payload,
    updated_at = NOW();

INSERT INTO kb.search_artifacts (
    artifact_type,
    artifact_id,
    input_record_id,
    source_row_id,
    primary_label,
    secondary_label,
    search_document,
    search_vector,
    snippet_basis,
    source_title,
    source_filename,
    category_paths,
    source_line_spans,
    semantic_payload,
    updated_at
)
SELECT
    'product' AS artifact_type,
    p.input_record_id::text || '_prd_' || COALESCE(NULLIF(regexp_replace(COALESCE(p.product_rel_id, ''), '^.*_', ''), ''), p.id::text) AS artifact_id,
    p.input_record_id,
    p.id AS source_row_id,
    COALESCE(NULLIF(p.product_name, ''), NULLIF(p.canonical_name, ''), 'Product #' || p.id::text) AS primary_label,
    COALESCE(NULLIF(p.product_type, ''), '') AS secondary_label,
    COALESCE(p.search_document, '') AS search_document,
    COALESCE(p.search_vector, to_tsvector('simple', COALESCE(p.search_document, ''))) AS search_vector,
    COALESCE(NULLIF(p.relation_summary, ''), NULLIF(p.product_name, ''), '') AS snippet_basis,
    COALESCE(i.file_name, '') AS source_title,
    COALESCE(i.file_name, '') AS source_filename,
    COALESCE(p.category_paths, '[]'::jsonb) AS category_paths,
    COALESCE(p.evidence_lines, '[]'::jsonb) AS source_line_spans,
    jsonb_build_object(
        'product_type', COALESCE(p.product_type, ''),
        'relation_type', COALESCE(p.relation_type, ''),
        'relation_summary', COALESCE(p.relation_summary, '')
    ) AS semantic_payload,
    NOW() AS updated_at
FROM kb.products p
JOIN kb.inputs i ON i.id = p.input_record_id
ON CONFLICT (artifact_type, artifact_id) DO UPDATE SET
    input_record_id = EXCLUDED.input_record_id,
    source_row_id = EXCLUDED.source_row_id,
    primary_label = EXCLUDED.primary_label,
    secondary_label = EXCLUDED.secondary_label,
    search_document = EXCLUDED.search_document,
    search_vector = EXCLUDED.search_vector,
    snippet_basis = EXCLUDED.snippet_basis,
    source_title = EXCLUDED.source_title,
    source_filename = EXCLUDED.source_filename,
    category_paths = EXCLUDED.category_paths,
    source_line_spans = EXCLUDED.source_line_spans,
    semantic_payload = EXCLUDED.semantic_payload,
    updated_at = NOW();

INSERT INTO kb.search_artifacts (
    artifact_type,
    artifact_id,
    input_record_id,
    source_row_id,
    primary_label,
    secondary_label,
    search_document,
    search_vector,
    snippet_basis,
    source_title,
    source_filename,
    category_paths,
    source_line_spans,
    semantic_payload,
    updated_at
)
SELECT
    'scene_block' AS artifact_type,
    s.input_record_id::text || '_sbk_' || COALESCE(NULLIF(regexp_replace(COALESCE(s.object_id, ''), '^.*_', ''), ''), s.id::text) AS artifact_id,
    s.input_record_id,
    s.id AS source_row_id,
    COALESCE(NULLIF(s.title, ''), 'Scene #' || s.id::text) AS primary_label,
    COALESCE(NULLIF(s.scene_type, ''), '') AS secondary_label,
    COALESCE(s.search_document, '') AS search_document,
    COALESCE(s.search_vector, to_tsvector('simple', COALESCE(s.search_document, ''))) AS search_vector,
    COALESCE(NULLIF(s.summary, ''), NULLIF(s.title, ''), '') AS snippet_basis,
    COALESCE(i.file_name, '') AS source_title,
    COALESCE(i.file_name, '') AS source_filename,
    '[]'::jsonb AS category_paths,
    COALESCE(s.source_refs, '[]'::jsonb) AS source_line_spans,
    jsonb_build_object(
        'scene_type', COALESCE(s.scene_type, ''),
        'summary', COALESCE(s.summary, ''),
        'keywords', COALESCE(s.keywords, '[]'::jsonb)
    ) AS semantic_payload,
    NOW() AS updated_at
FROM kb.scene_objects s
JOIN kb.inputs i ON i.id = s.input_record_id
ON CONFLICT (artifact_type, artifact_id) DO UPDATE SET
    input_record_id = EXCLUDED.input_record_id,
    source_row_id = EXCLUDED.source_row_id,
    primary_label = EXCLUDED.primary_label,
    secondary_label = EXCLUDED.secondary_label,
    search_document = EXCLUDED.search_document,
    search_vector = EXCLUDED.search_vector,
    snippet_basis = EXCLUDED.snippet_basis,
    source_title = EXCLUDED.source_title,
    source_filename = EXCLUDED.source_filename,
    category_paths = EXCLUDED.category_paths,
    source_line_spans = EXCLUDED.source_line_spans,
    semantic_payload = EXCLUDED.semantic_payload,
    updated_at = NOW();

-- +goose Down
DELETE FROM kb.search_artifacts
WHERE artifact_type IN ('metric', 'provision', 'product', 'scene_block');
