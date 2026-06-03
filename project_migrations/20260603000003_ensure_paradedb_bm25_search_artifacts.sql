-- +goose Up
-- Retry/ensure ParadeDB BM25 setup after pg_search becomes available.
--
-- Migration 20260603000002 is intentionally non-blocking when pg_search is not
-- available, so it may have been marked applied before the Nix Postgres package
-- was rebuilt with pg_search. This follow-up migration is idempotent and creates
-- the extension + per-partition BM25 indexes once pg_search is available.

-- +goose StatementBegin
DO $$
DECLARE
    part     text;
    idx_name text;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'pg_search') THEN
        RAISE NOTICE 'pg_search extension is not available; skipping ParadeDB BM25 indexes for kb.search_artifacts';
        RETURN;
    END IF;

    CREATE EXTENSION IF NOT EXISTS pg_search;

    FOR part IN
        SELECT inhrelid::regclass::text
        FROM pg_inherits
        WHERE inhparent = 'kb.search_artifacts'::regclass
    LOOP
        idx_name := replace(part, '.', '_') || '_bm25';
        EXECUTE format(
            $fmt$
            CREATE INDEX IF NOT EXISTS %I ON %s
            USING bm25 (
                artifact_id,
                search_document,
                primary_label,
                secondary_label,
                snippet_basis,
                source_title,
                source_filename
            )
            WITH (
                key_field = 'artifact_id',
                text_fields = '{
                    "search_document":  {"tokenizer": {"type": "jieba"}},
                    "primary_label":    {"tokenizer": {"type": "jieba"}},
                    "secondary_label":  {"tokenizer": {"type": "jieba"}},
                    "snippet_basis":    {"tokenizer": {"type": "jieba"}},
                    "source_title":     {"tokenizer": {"type": "jieba"}},
                    "source_filename":  {"tokenizer": {"type": "jieba"}}
                }'
            )
            $fmt$,
            idx_name, part
        );
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE
    part     text;
    idx_name text;
BEGIN
    FOR part IN
        SELECT inhrelid::regclass::text
        FROM pg_inherits
        WHERE inhparent = 'kb.search_artifacts'::regclass
    LOOP
        idx_name := replace(part, '.', '_') || '_bm25';
        EXECUTE format('DROP INDEX IF EXISTS kb.%I', idx_name);
    END LOOP;
END $$;
-- +goose StatementEnd

-- NOTE: intentionally NOT dropping pg_search because it is a server-level
-- facility and other objects may depend on it.
