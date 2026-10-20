-- +goose Up
-- Record how each agent run was executed. Every run before this migration
-- ran on the Pi harness.
ALTER TABLE kb.agentic_response_attempts
    ADD COLUMN IF NOT EXISTS harness         TEXT NOT NULL DEFAULT 'pi',
    ADD COLUMN IF NOT EXISTS adapter_version TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS model_name      TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE kb.agentic_response_attempts
    DROP COLUMN IF EXISTS model_name,
    DROP COLUMN IF EXISTS adapter_version,
    DROP COLUMN IF EXISTS harness;
