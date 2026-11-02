-- +goose Up
-- event_kind separates provider requests ('request') from doc-processor peak
-- control lifecycle rows: a held LLM call writes 'llm_hold', then 'llm_resume'
-- or 'llm_hold_cancelled'. Usage aggregates count only 'request' rows.
ALTER TABLE llm_usage_event
    ADD COLUMN IF NOT EXISTS event_kind TEXT NOT NULL DEFAULT 'request';

ALTER TABLE llm_usage_event DROP CONSTRAINT IF EXISTS llm_usage_event_event_kind_check;
ALTER TABLE llm_usage_event
    ADD CONSTRAINT llm_usage_event_event_kind_check
    CHECK (event_kind IN ('request', 'llm_hold', 'llm_resume', 'llm_hold_cancelled'));

-- +goose Down
ALTER TABLE llm_usage_event DROP CONSTRAINT IF EXISTS llm_usage_event_event_kind_check;
ALTER TABLE llm_usage_event DROP COLUMN IF EXISTS event_kind;
