-- +goose Up
CREATE TABLE IF NOT EXISTS kb.doc_review_requests (
    id              BIGSERIAL       PRIMARY KEY,
    input_record_id BIGINT          NOT NULL,
    review_run_id   TEXT,
    tier            TEXT            NOT NULL,
    aspects         JSONB           NOT NULL,
    reference_docs  JSONB,
    notes           TEXT,
    model_overrides JSONB,
    requester_name  TEXT            NOT NULL,
    requester_id    BIGINT          NOT NULL,
    report_template TEXT,
    doc_template    TEXT,
    status          TEXT            NOT NULL DEFAULT 'accepted',
    create_time     TIMESTAMPTZ     NOT NULL DEFAULT NOW(),
    start_time      TIMESTAMPTZ,
    end_time        TIMESTAMPTZ,
    error_message   TEXT
);

CREATE INDEX IF NOT EXISTS idx_doc_review_requests_record ON kb.doc_review_requests (input_record_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_requests_status ON kb.doc_review_requests (status);

-- +goose Down
DROP INDEX IF EXISTS idx_doc_review_requests_status;
DROP INDEX IF EXISTS idx_doc_review_requests_record;
DROP TABLE IF EXISTS kb.doc_review_requests;
