-- +goose Up
-- Makes the external-terminology staging tables syncable to deployment
-- machines (Sync Data page / server/api/datasync). A sync item needs
--   1. a natural key: a UNIQUE constraint other than the primary key (the
--      Sync Data picker and the registry only accept those), and
--   2. a timestamp cursor column.
--
-- These tables are immutable (see 20260807000001): rows are only ever
-- inserted, so the cursor is an insert time. Each natural key below repeats
-- the table's composite primary key (its stable business identity) as a named
-- UNIQUE constraint; the sync upsert uses ON CONFLICT DO NOTHING, so the extra
-- index adds no write conflicts and the immutability triggers never fire.
-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'keyword_sources_sync_natural_key'
          AND conrelid = 'kb.keyword_sources'::regclass
    ) THEN
        ALTER TABLE kb.keyword_sources
            ADD CONSTRAINT keyword_sources_sync_natural_key UNIQUE (source, release);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'keyword_source_artifacts_sync_natural_key'
          AND conrelid = 'kb.keyword_source_artifacts'::regclass
    ) THEN
        ALTER TABLE kb.keyword_source_artifacts
            ADD CONSTRAINT keyword_source_artifacts_sync_natural_key UNIQUE (source, release, artifact_id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'keyword_catalog_entries_sync_natural_key'
          AND conrelid = 'kb.keyword_catalog_entries'::regclass
    ) THEN
        ALTER TABLE kb.keyword_catalog_entries
            ADD CONSTRAINT keyword_catalog_entries_sync_natural_key UNIQUE (source, release, external_id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'keyword_catalog_labels_sync_natural_key'
          AND conrelid = 'kb.keyword_catalog_labels'::regclass
    ) THEN
        ALTER TABLE kb.keyword_catalog_labels
            ADD CONSTRAINT keyword_catalog_labels_sync_natural_key
            UNIQUE (source, release, external_id, language, label_role, label);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'keyword_catalog_relations_sync_natural_key'
          AND conrelid = 'kb.keyword_catalog_relations'::regclass
    ) THEN
        ALTER TABLE kb.keyword_catalog_relations
            ADD CONSTRAINT keyword_catalog_relations_sync_natural_key
            UNIQUE (source, release, subject_external_id, relation, object_source, object_release, object_external_id);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'keyword_catalog_negative_decisions_sync_natural_key'
          AND conrelid = 'kb.keyword_catalog_negative_decisions'::regclass
    ) THEN
        ALTER TABLE kb.keyword_catalog_negative_decisions
            ADD CONSTRAINT keyword_catalog_negative_decisions_sync_natural_key
            UNIQUE (source, release, subject_external_id, object_source, object_release, object_external_id, relation);
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conname = 'keyword_ucum_codes_sync_natural_key'
          AND conrelid = 'kb.keyword_ucum_codes'::regclass
    ) THEN
        ALTER TABLE kb.keyword_ucum_codes
            ADD CONSTRAINT keyword_ucum_codes_sync_natural_key UNIQUE (source, release, code);
    END IF;
END
$$;
-- +goose StatementEnd

-- Insert-time cursor. kb.keyword_source_artifacts already has created_at; the
-- other six tables have no timestamp (keyword_sources' retrieved_at is
-- nullable and describes the download, not the insert). now() is STABLE, so
-- existing rows get one backfilled value without a table rewrite, and ADD
-- COLUMN does not fire the immutability triggers.
ALTER TABLE kb.keyword_sources                    ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE kb.keyword_catalog_entries            ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE kb.keyword_catalog_labels             ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE kb.keyword_catalog_relations          ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE kb.keyword_catalog_negative_decisions ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();
ALTER TABLE kb.keyword_ucum_codes                 ADD COLUMN IF NOT EXISTS created_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- +goose Down
ALTER TABLE kb.keyword_ucum_codes                 DROP COLUMN IF EXISTS created_at;
ALTER TABLE kb.keyword_catalog_negative_decisions DROP COLUMN IF EXISTS created_at;
ALTER TABLE kb.keyword_catalog_relations          DROP COLUMN IF EXISTS created_at;
ALTER TABLE kb.keyword_catalog_labels             DROP COLUMN IF EXISTS created_at;
ALTER TABLE kb.keyword_catalog_entries            DROP COLUMN IF EXISTS created_at;
ALTER TABLE kb.keyword_sources                    DROP COLUMN IF EXISTS created_at;

ALTER TABLE kb.keyword_ucum_codes
    DROP CONSTRAINT IF EXISTS keyword_ucum_codes_sync_natural_key;
ALTER TABLE kb.keyword_catalog_negative_decisions
    DROP CONSTRAINT IF EXISTS keyword_catalog_negative_decisions_sync_natural_key;
ALTER TABLE kb.keyword_catalog_relations
    DROP CONSTRAINT IF EXISTS keyword_catalog_relations_sync_natural_key;
ALTER TABLE kb.keyword_catalog_labels
    DROP CONSTRAINT IF EXISTS keyword_catalog_labels_sync_natural_key;
ALTER TABLE kb.keyword_catalog_entries
    DROP CONSTRAINT IF EXISTS keyword_catalog_entries_sync_natural_key;
ALTER TABLE kb.keyword_source_artifacts
    DROP CONSTRAINT IF EXISTS keyword_source_artifacts_sync_natural_key;
ALTER TABLE kb.keyword_sources
    DROP CONSTRAINT IF EXISTS keyword_sources_sync_natural_key;
