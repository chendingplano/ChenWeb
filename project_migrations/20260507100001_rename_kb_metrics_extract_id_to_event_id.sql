-- +goose Up
-- Guarded (2026-07-25): on a fresh database, 20260401000000_create_kb_baseline_
-- tables.sql now creates kb.metrics with the column already named event_id
-- (nullable), so extract_id never exists there and this rename must no-op.
-- This file is already applied on every pre-existing database (Mac dev, staging
-- boxes), so editing its content has no effect on them — goose never re-runs or
-- diffs an already-applied migration.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'kb' AND table_name = 'metrics' AND column_name = 'extract_id'
    ) THEN
        ALTER TABLE kb.metrics RENAME COLUMN extract_id TO event_id;
        ALTER TABLE kb.metrics ALTER COLUMN event_id DROP NOT NULL;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE kb.metrics
    ALTER COLUMN event_id SET NOT NULL;
ALTER TABLE kb.metrics
    RENAME COLUMN event_id TO extract_id;
