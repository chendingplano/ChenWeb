-- +goose Up

CREATE TABLE IF NOT EXISTS ap_agent_trace (
    id                      VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    workspace_id            VARCHAR(64) NOT NULL
        REFERENCES ap_workspace(id) ON DELETE CASCADE,
    task_run_id             VARCHAR(64) NOT NULL
        REFERENCES ap_task_run(id) ON DELETE CASCADE,
    agent_kind              TEXT        NOT NULL,
    provider_trace_id       TEXT        NOT NULL DEFAULT '',
    input_text              TEXT        NOT NULL DEFAULT '',
    output_text             TEXT        NOT NULL DEFAULT '',
    tool_call_count         BIGINT      NOT NULL DEFAULT 0,
    input_tokens            BIGINT      NOT NULL DEFAULT 0,
    cached_input_tokens     BIGINT      NOT NULL DEFAULT 0,
    output_tokens           BIGINT      NOT NULL DEFAULT 0,
    reasoning_output_tokens BIGINT      NOT NULL DEFAULT 0,
    total_tokens            BIGINT      NOT NULL DEFAULT 0,
    total_latency_ms        BIGINT      NULL,
    total_cost_usd          DOUBLE PRECISION NULL,
    trace_json              JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ap_agent_trace_run_uidx UNIQUE (task_run_id)
);

CREATE INDEX IF NOT EXISTS ap_agent_trace_workspace_created_idx
    ON ap_agent_trace(workspace_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ap_agent_trace_agent_kind_idx
    ON ap_agent_trace(workspace_id, agent_kind, created_at DESC);
CREATE INDEX IF NOT EXISTS ap_agent_trace_total_tokens_idx
    ON ap_agent_trace(workspace_id, total_tokens DESC);

-- +goose Down

DROP INDEX IF EXISTS ap_agent_trace_total_tokens_idx;
DROP INDEX IF EXISTS ap_agent_trace_agent_kind_idx;
DROP INDEX IF EXISTS ap_agent_trace_workspace_created_idx;
DROP TABLE IF EXISTS ap_agent_trace;

