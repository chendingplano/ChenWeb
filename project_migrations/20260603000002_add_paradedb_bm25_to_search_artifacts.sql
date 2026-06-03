-- +goose Up
-- Adds ParadeDB BM25 lexical indexes for kb.search_artifacts.
--
-- This intentionally uses ParadeDB's modern SQL API:
--   CREATE EXTENSION pg_search
--   CREATE INDEX ... USING bm25 (...) WITH (key_field = 'artifact_id')
--   query path: field ||| $query, pdb.score(key_field)
--
-- Consequence of this choice: the database must run a ParadeDB/pg_search version
-- that exposes the modern `pdb` schema and `|||` operator. Older legacy-only
-- installs using `paradedb.*` query functions will not satisfy this migration.
-- The application defaults to PostgreSQL FTS unless SEARCH_LEXICAL_BACKEND=paradedb,
-- so non-ParadeDB dev/test environments continue to use the existing lexical path.
--
-- If pg_search is unavailable in this Postgres instance, skip the BM25 indexes
-- and leave the app on its PostgreSQL FTS fallback. This keeps doc processors
-- runnable on non-ParadeDB development databases while still enabling BM25
-- automatically on ParadeDB-capable databases.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_available_extensions WHERE name = 'pg_search') THEN
        CREATE EXTENSION IF NOT EXISTS pg_search;
    ELSE
        RAISE NOTICE 'pg_search extension is not available; skipping ParadeDB BM25 indexes for kb.search_artifacts';
    END IF;
END $$;
-- +goose StatementEnd

-- The parent table is LIST-partitioned by artifact_type. Create one BM25 index
-- per existing partition so each processor-owned artifact partition gets BM25
-- coverage and index builds remain isolated. New search_artifacts partitions
-- should add their own bm25 index in the migration that creates the partition.
--
-- artifact_id is unique inside each partition because kb.search_artifacts has
-- PRIMARY KEY (artifact_type, artifact_id), and each partition contains exactly
-- one artifact_type. That makes it a suitable ParadeDB key_field per partition.
--
-- Jieba is configured for the user-facing fields because the KnowledgeStore docs
-- mix English and Chinese text. ParadeDB tokenizes queries with the same tokenizer
-- by default, so this replaces the old CJK ILIKE fallback when the ParadeDB
-- backend is enabled.
-- +goose StatementBegin
DO $$
DECLARE
    part     text;
    idx_name text;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_search') THEN
        RAISE NOTICE 'pg_search extension is not installed; skipping ParadeDB BM25 indexes for kb.search_artifacts';
        RETURN;
    END IF;

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

-- NOTE: intentionally NOT dropping pg_search. It is a server-level extension and
-- other objects may come to depend on it; leaving it installed is harmless.
