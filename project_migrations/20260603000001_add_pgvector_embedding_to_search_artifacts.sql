-- +goose Up
-- Adds semantic-search support to kb.search_artifacts (pgvector interim of the
-- hybrid-search design in KnowledgeStore/Research/PostgreSQLIndex.typ).
--
-- Requires the pgvector ("vector") extension to be installed on the server.
-- On the Nix-managed Postgres this means adding pgvector to the postgresql
-- package set and restarting the server BEFORE this migration runs; otherwise
-- the CREATE EXTENSION below fails with "extension \"vector\" is not available".

CREATE EXTENSION IF NOT EXISTS vector;

-- Added on the partitioned parent -> propagates to all current partitions.
-- embedding_text stores the exact text that was embedded (for re-embedding / debug);
-- embedding holds the gpt-embedding-small (text-embedding-3-small) 1536-dim vector.
ALTER TABLE kb.search_artifacts
    ADD COLUMN IF NOT EXISTS embedding_text TEXT,
    ADD COLUMN IF NOT EXISTS embedding      vector(1536);

-- HNSW (cosine) index per partition. pgvector partitioned-parent indexes are
-- supported, but per-partition is consistent with the doc-processor capsule's
-- "Add New Doc Processor" checklist (each new partition creates its own indexes)
-- and keeps each index build isolated. New partitions added later must create
-- their own embedding HNSW index (see +CAPSULE.md section 3).
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
        idx_name := replace(part, '.', '_') || '_embedding_hnsw';
        EXECUTE format(
            'CREATE INDEX IF NOT EXISTS %I ON %s USING hnsw (embedding vector_cosine_ops)',
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
        idx_name := replace(part, '.', '_') || '_embedding_hnsw';
        EXECUTE format('DROP INDEX IF EXISTS kb.%I', idx_name);
    END LOOP;
END $$;
-- +goose StatementEnd

ALTER TABLE kb.search_artifacts
    DROP COLUMN IF EXISTS embedding,
    DROP COLUMN IF EXISTS embedding_text;

-- NOTE: intentionally NOT dropping the "vector" extension here. It is a shared
-- server facility that other objects may come to depend on; removing it in a
-- down-migration is riskier than leaving it installed (and harmless to keep).
