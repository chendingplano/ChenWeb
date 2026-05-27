-- +goose Up
ALTER TABLE kb.doc_proc_logs
    ADD COLUMN IF NOT EXISTS ms_used BIGINT;

UPDATE kb.doc_proc_logs
SET ms_used = COALESCE(
    ms_used,
    GREATEST(0, ROUND(EXTRACT(EPOCH FROM (end_time - start_time)) * 1000)::BIGINT)
)
WHERE end_time IS NOT NULL
  AND start_time IS NOT NULL;

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
