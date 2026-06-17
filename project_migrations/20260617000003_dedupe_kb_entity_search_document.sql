-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.entity_search_document(
    entity TEXT,
    entity_en TEXT,
    entity_type TEXT,
    entity_type_en TEXT,
    aliases JSONB,
    aliases_en JSONB,
    desc_text TEXT,
    desc_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    WITH parts(ord, subord, part) AS (
        SELECT * FROM (VALUES
            (1, 0, entity),
            (2, 0, entity_en),
            (3, 0, entity_type),
            (4, 0, entity_type_en)
        ) AS scalar_parts(ord, subord, part)
        UNION ALL
        SELECT 5, a.ord::int, a.value
        FROM jsonb_array_elements_text(
            CASE WHEN jsonb_typeof(aliases) = 'array' THEN aliases ELSE '[]'::jsonb END
        ) WITH ORDINALITY AS a(value, ord)
        UNION ALL
        SELECT 6, a.ord::int, a.value
        FROM jsonb_array_elements_text(
            CASE WHEN jsonb_typeof(aliases_en) = 'array' THEN aliases_en ELSE '[]'::jsonb END
        ) WITH ORDINALITY AS a(value, ord)
        UNION ALL
        SELECT * FROM (VALUES
            (7, 0, desc_text),
            (8, 0, desc_text_en)
        ) AS desc_parts(ord, subord, part)
        UNION ALL
        SELECT 9, a.ord::int, a.value
        FROM jsonb_array_elements_text(
            CASE WHEN jsonb_typeof(keywords) = 'array' THEN keywords ELSE '[]'::jsonb END
        ) WITH ORDINALITY AS a(value, ord)
        UNION ALL
        SELECT 10, a.ord::int, a.value
        FROM jsonb_array_elements_text(
            CASE WHEN jsonb_typeof(keywords_en) = 'array' THEN keywords_en ELSE '[]'::jsonb END
        ) WITH ORDINALITY AS a(value, ord)
    ),
    normalized AS (
        SELECT
            ord,
            subord,
            btrim(regexp_replace(COALESCE(part, ''), '\s+', ' ', 'g')) AS part
        FROM parts
    ),
    deduped AS (
        SELECT DISTINCT ON (lower(part))
            ord,
            subord,
            part
        FROM normalized
        WHERE part <> ''
        ORDER BY lower(part), ord, subord
    )
    SELECT COALESCE(string_agg(part, ' ' ORDER BY ord, subord), '')
    FROM deduped;
$$;
-- +goose StatementEnd

UPDATE kb.entities
SET
    entity_context = NULL,
    search_document = kb.entity_search_document(
        entity, entity_en,
        entity_type, entity_type_en,
        aliases, aliases_en,
        desc_text, desc_text_en,
        keywords, keywords_en
    ),
    search_vector = to_tsvector(
        'simple',
        kb.entity_search_document(
            entity, entity_en,
            entity_type, entity_type_en,
            aliases, aliases_en,
            desc_text, desc_text_en,
            keywords, keywords_en
        )
    );

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.entity_search_document(
    entity TEXT,
    entity_en TEXT,
    entity_type TEXT,
    entity_type_en TEXT,
    aliases JSONB,
    aliases_en JSONB,
    desc_text TEXT,
    desc_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB
)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
AS $$
    SELECT trim(concat_ws(
        ' ',
        COALESCE(entity, ''),
        COALESCE(entity_en, ''),
        COALESCE(entity_type, ''),
        COALESCE(entity_type_en, ''),
        kb.search_jsonb_array_text(aliases),
        kb.search_jsonb_array_text(aliases_en),
        COALESCE(desc_text, ''),
        COALESCE(desc_text_en, ''),
        kb.search_jsonb_array_text(keywords),
        kb.search_jsonb_array_text(keywords_en)
    ));
$$;
-- +goose StatementEnd

UPDATE kb.entities
SET
    search_document = kb.entity_search_document(
        entity, entity_en,
        entity_type, entity_type_en,
        aliases, aliases_en,
        desc_text, desc_text_en,
        keywords, keywords_en
    ),
    search_vector = to_tsvector(
        'simple',
        kb.entity_search_document(
            entity, entity_en,
            entity_type, entity_type_en,
            aliases, aliases_en,
            desc_text, desc_text_en,
            keywords, keywords_en
        )
    );
