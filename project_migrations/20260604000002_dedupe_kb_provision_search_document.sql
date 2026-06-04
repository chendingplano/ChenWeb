-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.provision_search_document(
    prov_name TEXT,
    prov_name_en TEXT,
    provision_type TEXT,
    source_text TEXT,
    provision TEXT,
    provision_en TEXT,
    provision_subject TEXT,
    provision_subject_en TEXT,
    prov_desc TEXT,
    prov_desc_en TEXT,
    prov_context TEXT,
    prov_context_en TEXT,
    provision_keywords JSONB,
    provision_keywords_en JSONB,
    category_paths JSONB,
    category_paths_en JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT kb.search_text_join_unique(
        COALESCE(prov_name, ''),
        COALESCE(prov_name_en, ''),
        COALESCE(provision_type, ''),
        COALESCE(source_text, ''),
        COALESCE(provision, ''),
        COALESCE(provision_en, ''),
        COALESCE(provision_subject, ''),
        COALESCE(provision_subject_en, ''),
        COALESCE(prov_desc, ''),
        COALESCE(prov_desc_en, ''),
        COALESCE(prov_context, ''),
        COALESCE(prov_context_en, ''),
        kb.search_jsonb_array_text(provision_keywords),
        kb.search_jsonb_array_text(provision_keywords_en),
        kb.search_jsonb_array_text(category_paths),
        kb.search_jsonb_array_text(category_paths_en)
    );
$$;
-- +goose StatementEnd

UPDATE kb.provisions
SET
    search_document = kb.provision_search_document(
        prov_name, prov_name_en,
        provision_type, source_text,
        provision, provision_en,
        provision_subject, provision_subject_en,
        prov_desc, prov_desc_en,
        prov_context, prov_context_en,
        provision_keywords, provision_keywords_en,
        category_paths, category_paths_en
    ),
    search_vector = to_tsvector(
        'simple',
        kb.provision_search_document(
            prov_name, prov_name_en,
            provision_type, source_text,
            provision, provision_en,
            provision_subject, provision_subject_en,
            prov_desc, prov_desc_en,
            prov_context, prov_context_en,
            provision_keywords, provision_keywords_en,
            category_paths, category_paths_en
        )
    );

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.provision_search_document(
    prov_name TEXT,
    prov_name_en TEXT,
    provision_type TEXT,
    source_text TEXT,
    provision TEXT,
    provision_en TEXT,
    provision_subject TEXT,
    provision_subject_en TEXT,
    prov_desc TEXT,
    prov_desc_en TEXT,
    prov_context TEXT,
    prov_context_en TEXT,
    provision_keywords JSONB,
    provision_keywords_en JSONB,
    category_paths JSONB,
    category_paths_en JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(prov_name, ''),
        COALESCE(prov_name_en, ''),
        COALESCE(provision_type, ''),
        COALESCE(source_text, ''),
        COALESCE(provision, ''),
        COALESCE(provision_en, ''),
        COALESCE(provision_subject, ''),
        COALESCE(provision_subject_en, ''),
        COALESCE(prov_desc, ''),
        COALESCE(prov_desc_en, ''),
        COALESCE(prov_context, ''),
        COALESCE(prov_context_en, ''),
        kb.search_jsonb_array_text(provision_keywords),
        kb.search_jsonb_array_text(provision_keywords_en),
        kb.search_jsonb_array_text(category_paths),
        kb.search_jsonb_array_text(category_paths_en)
    ));
$$;
-- +goose StatementEnd

UPDATE kb.provisions
SET
    search_document = kb.provision_search_document(
        prov_name, prov_name_en,
        provision_type, source_text,
        provision, provision_en,
        provision_subject, provision_subject_en,
        prov_desc, prov_desc_en,
        prov_context, prov_context_en,
        provision_keywords, provision_keywords_en,
        category_paths, category_paths_en
    ),
    search_vector = to_tsvector(
        'simple',
        kb.provision_search_document(
            prov_name, prov_name_en,
            provision_type, source_text,
            provision, provision_en,
            provision_subject, provision_subject_en,
            prov_desc, prov_desc_en,
            prov_context, prov_context_en,
            provision_keywords, provision_keywords_en,
            category_paths, category_paths_en
        )
    );
