-- +goose Up
CREATE TABLE IF NOT EXISTS kb.failed_llm_calls (
    id BIGSERIAL PRIMARY KEY,
    record_id BIGINT,
    run_id BIGINT,
    operation TEXT NOT NULL,
    prompt_name TEXT NOT NULL,
    primary_model TEXT NOT NULL,
    fallback_model TEXT NOT NULL,
    primary_error TEXT NOT NULL,
    raw_response TEXT NOT NULL DEFAULT '',
    chunk_index INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_failed_llm_calls_record_created
    ON kb.failed_llm_calls (record_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS kb.failed_llm_calls;
