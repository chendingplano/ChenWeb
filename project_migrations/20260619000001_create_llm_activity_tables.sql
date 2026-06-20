-- +goose Up

CREATE TABLE IF NOT EXISTS llm_account (
    id                         VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    account_name               TEXT        NOT NULL,
    provider                   TEXT        NOT NULL,
    base_url                   TEXT        NOT NULL DEFAULT '',
    api_key_ref                TEXT        NOT NULL DEFAULT '',
    status                     TEXT        NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'disabled')),
    reconciliation_kind        TEXT        NOT NULL DEFAULT '',
    is_reconciliation_enabled  BOOLEAN     NOT NULL DEFAULT FALSE,
    default_model_name         TEXT        NOT NULL DEFAULT '',
    metadata_json              JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS llm_account_name_uidx
    ON llm_account(LOWER(account_name));
CREATE INDEX IF NOT EXISTS llm_account_provider_idx
    ON llm_account(provider, created_at DESC);

CREATE TABLE IF NOT EXISTS llm_account_model_profile (
    id                         VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    account_id                 VARCHAR(64) NOT NULL
        REFERENCES llm_account(id) ON DELETE RESTRICT,
    profile_name               TEXT        NOT NULL,
    model_name                 TEXT        NOT NULL DEFAULT '',
    thinking_type              TEXT        NOT NULL DEFAULT '',
    timeout_sec                INT         NOT NULL DEFAULT 0,
    max_inflight               INT         NOT NULL DEFAULT 0,
    max_requests_per_minute    INT         NOT NULL DEFAULT 0,
    max_tokens_per_minute      INT         NOT NULL DEFAULT 0,
    token_reserve_per_call     INT         NOT NULL DEFAULT 0,
    is_active                  BOOLEAN     NOT NULL DEFAULT TRUE,
    metadata_json              JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS llm_account_model_profile_name_uidx
    ON llm_account_model_profile(account_id, LOWER(profile_name));
CREATE INDEX IF NOT EXISTS llm_account_model_profile_account_idx
    ON llm_account_model_profile(account_id, created_at DESC);

CREATE TABLE IF NOT EXISTS llm_usage_event (
    id                         VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    account_id                 VARCHAR(64) NOT NULL
        REFERENCES llm_account(id) ON DELETE RESTRICT,
    profile_id                 VARCHAR(64) NOT NULL
        REFERENCES llm_account_model_profile(id) ON DELETE RESTRICT,
    provider                   TEXT        NOT NULL,
    model_name                 TEXT        NOT NULL DEFAULT '',
    prompt_name                TEXT        NOT NULL DEFAULT '',
    request_started_at         TIMESTAMPTZ NOT NULL,
    request_finished_at        TIMESTAMPTZ NULL,
    workspace_day              DATE        NOT NULL,
    input_tokens               BIGINT      NOT NULL DEFAULT 0,
    output_tokens              BIGINT      NOT NULL DEFAULT 0,
    total_tokens               BIGINT      NOT NULL DEFAULT 0,
    latency_ms                 BIGINT      NOT NULL DEFAULT 0,
    http_status                INT         NOT NULL DEFAULT 0,
    error_message              TEXT        NOT NULL DEFAULT '',
    input_body_ref             TEXT        NOT NULL DEFAULT '',
    output_body_ref            TEXT        NOT NULL DEFAULT '',
    provider_request_id        TEXT        NOT NULL DEFAULT '',
    metadata_json              JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS llm_usage_event_account_day_idx
    ON llm_usage_event(account_id, workspace_day, request_started_at DESC);
CREATE INDEX IF NOT EXISTS llm_usage_event_profile_started_idx
    ON llm_usage_event(profile_id, request_started_at DESC);
CREATE INDEX IF NOT EXISTS llm_usage_event_started_idx
    ON llm_usage_event(request_started_at DESC);

CREATE TABLE IF NOT EXISTS llm_daily_account_report (
    id                         VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    account_id                 VARCHAR(64) NOT NULL
        REFERENCES llm_account(id) ON DELETE RESTRICT,
    workspace_day              DATE        NOT NULL,
    timezone_name              TEXT        NOT NULL DEFAULT '',
    opening_balance            DOUBLE PRECISION NOT NULL DEFAULT 0,
    closing_balance            DOUBLE PRECISION NOT NULL DEFAULT 0,
    spend_amount               DOUBLE PRECISION NOT NULL DEFAULT 0,
    currency_code              TEXT        NOT NULL DEFAULT 'USD',
    input_tokens               BIGINT      NOT NULL DEFAULT 0,
    output_tokens              BIGINT      NOT NULL DEFAULT 0,
    total_tokens               BIGINT      NOT NULL DEFAULT 0,
    request_count              BIGINT      NOT NULL DEFAULT 0,
    reconciliation_status      TEXT        NOT NULL DEFAULT 'pending',
    source_kind                TEXT        NOT NULL DEFAULT '',
    source_payload_ref         TEXT        NOT NULL DEFAULT '',
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT llm_daily_account_report_account_day_uidx
        UNIQUE (account_id, workspace_day)
);

CREATE INDEX IF NOT EXISTS llm_daily_account_report_day_idx
    ON llm_daily_account_report(workspace_day DESC);

CREATE TABLE IF NOT EXISTS llm_balance_snapshot (
    id                         VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    account_id                 VARCHAR(64) NOT NULL
        REFERENCES llm_account(id) ON DELETE RESTRICT,
    captured_at                TIMESTAMPTZ NOT NULL,
    workspace_day              DATE        NOT NULL,
    balance_amount             DOUBLE PRECISION NOT NULL DEFAULT 0,
    currency_code              TEXT        NOT NULL DEFAULT 'USD',
    raw_payload_ref            TEXT        NOT NULL DEFAULT '',
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS llm_balance_snapshot_account_captured_idx
    ON llm_balance_snapshot(account_id, captured_at DESC);
CREATE INDEX IF NOT EXISTS llm_balance_snapshot_day_idx
    ON llm_balance_snapshot(workspace_day DESC);

-- +goose Down

DROP INDEX IF EXISTS llm_balance_snapshot_day_idx;
DROP INDEX IF EXISTS llm_balance_snapshot_account_captured_idx;
DROP TABLE IF EXISTS llm_balance_snapshot;

DROP INDEX IF EXISTS llm_daily_account_report_day_idx;
DROP TABLE IF EXISTS llm_daily_account_report;

DROP INDEX IF EXISTS llm_usage_event_started_idx;
DROP INDEX IF EXISTS llm_usage_event_profile_started_idx;
DROP INDEX IF EXISTS llm_usage_event_account_day_idx;
DROP TABLE IF EXISTS llm_usage_event;

DROP INDEX IF EXISTS llm_account_model_profile_account_idx;
DROP INDEX IF EXISTS llm_account_model_profile_name_uidx;
DROP TABLE IF EXISTS llm_account_model_profile;

DROP INDEX IF EXISTS llm_account_provider_idx;
DROP INDEX IF EXISTS llm_account_name_uidx;
DROP TABLE IF EXISTS llm_account;
