-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS uq_agentic_one_running_attempt_per_conversation
    ON kb.agentic_response_attempts(conversation_id)
    WHERE status = 'running';

-- +goose Down
DROP INDEX IF EXISTS kb.uq_agentic_one_running_attempt_per_conversation;
