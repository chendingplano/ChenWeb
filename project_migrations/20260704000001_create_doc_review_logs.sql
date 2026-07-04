-- +goose Up
CREATE TABLE IF NOT EXISTS kb.doc_review_logs (
    id              BIGSERIAL    PRIMARY KEY,
    input_record_id BIGINT       NOT NULL,
    run_id          BIGINT       NOT NULL,
    pass            TEXT         NOT NULL,
    aspect          TEXT         NOT NULL,
    unit_type       TEXT         NOT NULL,
    unit_key        TEXT         NOT NULL,
    unit_location   JSONB,
    matched_units   JSONB,
    findings        JSONB,
    outcome         TEXT         NOT NULL,
    detail          JSONB,
    create_time     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, aspect, unit_type, unit_key)
);

CREATE INDEX IF NOT EXISTS idx_doc_review_logs_record  ON kb.doc_review_logs (input_record_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_logs_run     ON kb.doc_review_logs (run_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_logs_aspect  ON kb.doc_review_logs (input_record_id, aspect);
CREATE INDEX IF NOT EXISTS idx_doc_review_logs_outcome ON kb.doc_review_logs (run_id, outcome);

-- +goose Down
DROP TABLE IF EXISTS kb.doc_review_logs;
