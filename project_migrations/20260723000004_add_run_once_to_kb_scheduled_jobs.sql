-- +goose Up
ALTER TABLE kb.scheduled_jobs
    ADD COLUMN IF NOT EXISTS run_once BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE kb.scheduled_jobs
    DROP COLUMN IF EXISTS run_once;
