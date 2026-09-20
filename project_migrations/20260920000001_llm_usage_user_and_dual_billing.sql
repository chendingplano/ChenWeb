-- +goose Up

ALTER TABLE llm_usage_event ADD COLUMN IF NOT EXISTS user_id TEXT NULL;
CREATE INDEX IF NOT EXISTS llm_usage_event_user_started_idx
    ON llm_usage_event(user_id, request_started_at DESC);

ALTER TABLE llm_balance_snapshot
    ALTER COLUMN balance_amount TYPE NUMERIC(20, 6),
    ADD COLUMN IF NOT EXISTS capture_source TEXT NOT NULL DEFAULT 'manual';

CREATE INDEX IF NOT EXISTS llm_balance_snapshot_account_currency_captured_idx
    ON llm_balance_snapshot(account_id, currency_code, captured_at DESC);

CREATE TABLE IF NOT EXISTS llm_balance_capture_slot (
    account_id VARCHAR(64) NOT NULL REFERENCES llm_account(id) ON DELETE RESTRICT,
    scheduled_hour TIMESTAMPTZ NOT NULL,
    claimed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, scheduled_hour),
    UNIQUE (account_id, scheduled_hour)
);

ALTER TABLE llm_account ADD COLUMN IF NOT EXISTS api_key_display_fingerprint TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS llm_official_balance_delta_report (
    id VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    account_id VARCHAR(64) NOT NULL REFERENCES llm_account(id) ON DELETE RESTRICT,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    currency_code TEXT NOT NULL,
    opening_balance NUMERIC(20, 6),
    closing_balance NUMERIC(20, 6),
    balance_delta NUMERIC(20, 6),
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, period_start, period_end, currency_code)
);

CREATE TABLE IF NOT EXISTS llm_local_model_cost_report (
    id VARCHAR(64) PRIMARY KEY DEFAULT gen_random_uuid()::text,
    account_id VARCHAR(64) NOT NULL REFERENCES llm_account(id) ON DELETE RESTRICT,
    model_name TEXT NOT NULL,
    period_start TIMESTAMPTZ NOT NULL,
    period_end TIMESTAMPTZ NOT NULL,
    currency_code TEXT NOT NULL DEFAULT 'CNY',
    input_cache_hit_cost NUMERIC(20, 6) NOT NULL DEFAULT 0,
    input_cache_miss_cost NUMERIC(20, 6) NOT NULL DEFAULT 0,
    output_cost NUMERIC(20, 6) NOT NULL DEFAULT 0,
    total_cost NUMERIC(20, 6) NOT NULL DEFAULT 0,
    status TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, model_name, period_start, period_end, currency_code)
);

-- +goose Down

DROP TABLE IF EXISTS llm_local_model_cost_report;
DROP TABLE IF EXISTS llm_official_balance_delta_report;
ALTER TABLE llm_account DROP COLUMN IF EXISTS api_key_display_fingerprint;
DROP TABLE IF EXISTS llm_balance_capture_slot;
DROP INDEX IF EXISTS llm_balance_snapshot_account_currency_captured_idx;
ALTER TABLE llm_balance_snapshot DROP COLUMN IF EXISTS capture_source;
ALTER TABLE llm_usage_event DROP COLUMN IF EXISTS user_id;
