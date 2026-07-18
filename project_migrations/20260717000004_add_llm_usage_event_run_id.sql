-- +goose Up

ALTER TABLE llm_usage_event
    ADD COLUMN IF NOT EXISTS run_id BIGINT NULL;

CREATE INDEX IF NOT EXISTS llm_usage_event_run_id_started_idx
    ON llm_usage_event(run_id, request_started_at DESC);

-- +goose Down

DROP INDEX IF EXISTS llm_usage_event_run_id_started_idx;

ALTER TABLE llm_usage_event
    DROP COLUMN IF EXISTS run_id;
