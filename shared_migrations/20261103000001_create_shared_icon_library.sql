-- +goose Up
-- Vector icon catalogue (Lucide base, Tabler gap-filler, custom SVG). Searched by
-- keywords or short descriptions in English and Chinese.
-- See shared/openspec/changes/shared-icon-library/design.md.
CREATE SCHEMA IF NOT EXISTS shared;

CREATE TABLE IF NOT EXISTS shared.icon_library (
    id              BIGSERIAL     PRIMARY KEY,
    name            VARCHAR(128)  NOT NULL,
    source_set      VARCHAR(16)   NOT NULL CHECK (source_set IN ('lucide', 'tabler', 'custom')),
    source_version  VARCHAR(32)   NOT NULL DEFAULT '',
    category_en     VARCHAR(128)  NOT NULL DEFAULT '',
    category_zh     VARCHAR(128)  NOT NULL DEFAULT '',
    tags_en         TEXT          NOT NULL DEFAULT '',
    tags_zh         TEXT          NOT NULL DEFAULT '',
    description_en  TEXT          NOT NULL DEFAULT '',
    description_zh  TEXT          NOT NULL DEFAULT '',
    svg_body        TEXT          NOT NULL,
    version         INT           NOT NULL DEFAULT 1,
    embedding       public.vector(1024)  NULL,
    creator         VARCHAR(255)  NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_icon_library_name UNIQUE (name)
);

CREATE INDEX IF NOT EXISTS idx_icon_library_category ON shared.icon_library (category_en);

-- Vector index is skipped when pgvector is absent; search then falls back to BM25.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'vector') THEN
        EXECUTE 'CREATE INDEX IF NOT EXISTS idx_icon_library_embedding
                 ON shared.icon_library USING hnsw (embedding public.vector_cosine_ops)';
    END IF;

    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'pg_search') THEN
        CREATE EXTENSION IF NOT EXISTS pg_search;
        EXECUTE $fmt$
            CREATE INDEX IF NOT EXISTS idx_icon_library_bm25 ON shared.icon_library
            USING bm25 (id, name, category_en, category_zh, tags_en, tags_zh,
                        description_en, description_zh)
            WITH (
                key_field = 'id',
                text_fields = '{
                    "category_zh":    {"tokenizer": {"type": "jieba"}},
                    "tags_zh":        {"tokenizer": {"type": "jieba"}},
                    "description_zh": {"tokenizer": {"type": "jieba"}}
                }'
            )
        $fmt$;
    ELSE
        RAISE NOTICE 'pg_search not available; skipping BM25 index on shared.icon_library';
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE IF EXISTS shared.icon_library;
