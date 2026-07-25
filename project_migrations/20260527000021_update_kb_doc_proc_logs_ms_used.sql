-- +goose Up
-- Guarded (2026-07-25): start_time/end_time predate this migration directory
-- (like kb.inputs etc., see 20260401000000_create_kb_baseline_tables.sql) and
-- 20260527000020_create_kb_doc_proc_logs.sql never created them, so on a
-- truly fresh database they never exist and this backfill must no-op. This
-- file is already applied on every pre-existing database, so editing its
-- content has no effect there — goose never re-runs an applied migration.
ALTER TABLE kb.doc_proc_logs
    ADD COLUMN IF NOT EXISTS ms_used BIGINT;

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'kb' AND table_name = 'doc_proc_logs' AND column_name = 'start_time'
    ) AND EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'kb' AND table_name = 'doc_proc_logs' AND column_name = 'end_time'
    ) THEN
        UPDATE kb.doc_proc_logs
        SET ms_used = COALESCE(
            ms_used,
            GREATEST(0, ROUND(EXTRACT(EPOCH FROM (end_time - start_time)) * 1000)::BIGINT)
        )
        WHERE end_time IS NOT NULL
          AND start_time IS NOT NULL;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE kb.doc_proc_logs
    DROP COLUMN IF EXISTS start_time,
    DROP COLUMN IF EXISTS end_time;

-- +goose Down
ALTER TABLE kb.doc_proc_logs
    ADD COLUMN IF NOT EXISTS start_time TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS end_time TIMESTAMPTZ;

UPDATE kb.doc_proc_logs
SET end_time = COALESCE(end_time, create_time),
    start_time = COALESCE(
        start_time,
        create_time - make_interval(secs => COALESCE(ms_used, 0)::double precision / 1000.0)
    );

ALTER TABLE kb.doc_proc_logs
    ALTER COLUMN start_time SET NOT NULL,
    ALTER COLUMN end_time SET NOT NULL;

ALTER TABLE kb.doc_proc_logs
    DROP COLUMN IF EXISTS ms_used;
