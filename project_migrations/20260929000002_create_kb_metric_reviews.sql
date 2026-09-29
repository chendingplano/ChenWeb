-- +goose Up
-- openspec change llm-review-metrics: one row per LLM review run of a record's
-- extracted kb.metrics (System Admin -> LLM -> Review Metrics). A record's current
-- review is its newest row; forced re-reviews insert new rows so history is kept.
-- status: 'running' | 'done' | 'failed'. report is NULL until status = 'done'.
CREATE TABLE IF NOT EXISTS kb.metric_reviews (
    id               BIGSERIAL    PRIMARY KEY,
    input_record_id  BIGINT       NOT NULL,
    status           TEXT         NOT NULL,
    report           JSONB,
    error_msg        TEXT,
    model_name       TEXT,
    prompt_name      TEXT,
    metrics_count    INT          NOT NULL DEFAULT 0,
    created_by       TEXT,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    finished_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_kb_metric_reviews_record
    ON kb.metric_reviews (input_record_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_metric_reviews_record;
DROP TABLE IF EXISTS kb.metric_reviews;
