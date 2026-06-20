-- +goose Up

ALTER TABLE llm_usage_event
    ADD COLUMN IF NOT EXISTS record_id BIGINT NULL,
    ADD COLUMN IF NOT EXISTS call_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS call_loc TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS llm_usage_event_record_started_idx
    ON llm_usage_event(record_id, request_started_at DESC);

CREATE INDEX IF NOT EXISTS llm_usage_event_call_loc_started_idx
    ON llm_usage_event(call_loc, request_started_at DESC);

-- +goose Down

DROP INDEX IF EXISTS llm_usage_event_call_loc_started_idx;
DROP INDEX IF EXISTS llm_usage_event_record_started_idx;

ALTER TABLE llm_usage_event
    DROP COLUMN IF EXISTS call_loc,
    DROP COLUMN IF EXISTS call_reason,
    DROP COLUMN IF EXISTS record_id;
