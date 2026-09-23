-- +goose Up

-- MID_26092201/MID_26092202: an LLM call can fail at the schema/parse layer
-- (structured output retries exhausted, response truncated by the provider's
-- max_tokens) after the transport layer already saw HTTP 2xx, so no error was
-- visible on the per-attempt usage event and the actual response content was
-- only reachable via the gzip-archived output_body_ref. raw_response stores
-- that content inline so a failed call is queryable directly off this table.
ALTER TABLE llm_usage_event ADD COLUMN IF NOT EXISTS raw_response TEXT NOT NULL DEFAULT '';

-- +goose Down

ALTER TABLE llm_usage_event DROP COLUMN IF EXISTS raw_response;
