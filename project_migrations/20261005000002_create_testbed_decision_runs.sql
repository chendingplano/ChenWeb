-- +goose Up
-- Every run of the Decision Model Playground (requirement 2026100502-rqmt):
-- its full input (model, exact policy text, text judged, questions) and its
-- outcome (answers, raw provider response, or the error). Failed runs are
-- kept too, so a model or policy that errors can be investigated.
CREATE SCHEMA IF NOT EXISTS testbed;

CREATE TABLE IF NOT EXISTS testbed.decision_runs (
    id              BIGSERIAL   PRIMARY KEY,
    model_key       TEXT        NOT NULL,
    model_name      TEXT        NOT NULL,
    provider        TEXT        NOT NULL,
    -- shared.decision_policies.id / version the policy text started from
    -- (NULL when no stored policy was selected). policy_content is the exact
    -- text sent; policy_edited says it differed from that stored version.
    policy_id       BIGINT,
    policy_version  INT,
    policy_edited   BOOLEAN     NOT NULL DEFAULT FALSE,
    policy_content  TEXT        NOT NULL DEFAULT '',
    input_text      TEXT        NOT NULL DEFAULT '',
    questions       JSONB       NOT NULL,
    run_status      TEXT        NOT NULL CHECK (run_status IN ('ok', 'error')),
    answers         JSONB,
    raw_response    JSONB,
    error_message   TEXT        NOT NULL DEFAULT '',
    input_tokens    INT         NOT NULL DEFAULT 0,
    output_tokens   INT         NOT NULL DEFAULT 0,
    elapsed_ms      BIGINT      NOT NULL DEFAULT 0,
    -- public.llm_usage_event ids, one per provider call
    usage_event_ids JSONB       NOT NULL DEFAULT '[]'::jsonb,
    created_by      TEXT        NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS decision_runs_created_idx
    ON testbed.decision_runs (created_at DESC);
CREATE INDEX IF NOT EXISTS decision_runs_policy_idx
    ON testbed.decision_runs (policy_id, policy_version);

-- +goose Down
DROP INDEX IF EXISTS testbed.decision_runs_policy_idx;
DROP INDEX IF EXISTS testbed.decision_runs_created_idx;
DROP TABLE IF EXISTS testbed.decision_runs;
