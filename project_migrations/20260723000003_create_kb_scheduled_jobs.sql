-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.scheduled_jobs (
    id               BIGSERIAL PRIMARY KEY,
    name             TEXT NOT NULL,
    job_type         TEXT NOT NULL,
    interval_seconds INT NOT NULL CHECK (interval_seconds > 0),
    params           JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled          BOOLEAN NOT NULL DEFAULT true,
    next_run_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_run_at      TIMESTAMPTZ,
    last_run_status  TEXT,
    create_time      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_scheduled_jobs_due
    ON kb.scheduled_jobs (enabled, next_run_at);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.touch_scheduled_jobs_modify_time()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.modify_time := NOW();
    RETURN NEW;
END;
$$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS trg_touch_scheduled_jobs_modify_time ON kb.scheduled_jobs;
CREATE TRIGGER trg_touch_scheduled_jobs_modify_time
BEFORE UPDATE ON kb.scheduled_jobs
FOR EACH ROW
EXECUTE FUNCTION kb.touch_scheduled_jobs_modify_time();

CREATE TABLE IF NOT EXISTS kb.scheduled_job_runs (
    id           BIGSERIAL PRIMARY KEY,
    schedule_id  BIGINT NOT NULL REFERENCES kb.scheduled_jobs(id) ON DELETE CASCADE,
    job_type     TEXT NOT NULL,
    status       TEXT NOT NULL CHECK (status IN ('running', 'success', 'failed')),
    started_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at  TIMESTAMPTZ,
    result       JSONB,
    error        TEXT,
    create_time  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_scheduled_job_runs_schedule
    ON kb.scheduled_job_runs (schedule_id, started_at DESC);

-- +goose Down
DROP TABLE IF EXISTS kb.scheduled_job_runs;
DROP TRIGGER IF EXISTS trg_touch_scheduled_jobs_modify_time ON kb.scheduled_jobs;
DROP FUNCTION IF EXISTS kb.touch_scheduled_jobs_modify_time();
DROP INDEX IF EXISTS kb.idx_kb_scheduled_jobs_due;
DROP TABLE IF EXISTS kb.scheduled_jobs;
