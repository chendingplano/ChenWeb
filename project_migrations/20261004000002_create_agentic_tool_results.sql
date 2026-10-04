-- +goose Up
-- Full tool-call records for Knowledge Desk conversations: the arguments and
-- the exact response ChenWeb returned to Pi. Replayed (possibly shortened) in
-- later turns' history and returned in full by get_saved_tool_result.
CREATE TABLE IF NOT EXISTS kb.agentic_tool_results (
    id                   BIGSERIAL PRIMARY KEY,
    attempt_id           VARCHAR(64) NOT NULL REFERENCES kb.agentic_response_attempts(id) ON DELETE CASCADE,
    gateway_tool_call_id TEXT NOT NULL CHECK (BTRIM(gateway_tool_call_id) <> ''),
    tool_name            TEXT NOT NULL,
    arguments            JSONB NOT NULL DEFAULT '{}'::jsonb,
    result               TEXT NOT NULL,
    is_error             BOOLEAN NOT NULL DEFAULT FALSE,
    document_ids         TEXT[] NOT NULL DEFAULT '{}',
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (attempt_id, gateway_tool_call_id)
);

CREATE INDEX IF NOT EXISTS idx_agentic_tool_results_attempt
    ON kb.agentic_tool_results (attempt_id, id);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_agentic_tool_results_attempt;
DROP TABLE IF EXISTS kb.agentic_tool_results;
