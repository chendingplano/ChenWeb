-- +goose Up
CREATE TABLE IF NOT EXISTS kb.doc_review_reports (
    id                BIGSERIAL       PRIMARY KEY,
    request_id        BIGINT          NOT NULL,
    input_record_id   BIGINT          NOT NULL,
    review_run_id     TEXT            NOT NULL,
    report_json       JSONB           NOT NULL,
    report_markdown   TEXT            NOT NULL,
    executive_summary TEXT            NOT NULL,
    total_findings    INT             NOT NULL,
    high_count        INT             NOT NULL DEFAULT 0,
    medium_count      INT             NOT NULL DEFAULT 0,
    low_count         INT             NOT NULL DEFAULT 0,
    overall_assessment TEXT           NOT NULL,
    create_time       TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_doc_review_reports_request ON kb.doc_review_reports (request_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_reports_record  ON kb.doc_review_reports (input_record_id);

-- +goose Down
DROP INDEX IF EXISTS idx_doc_review_reports_record;
DROP INDEX IF EXISTS idx_doc_review_reports_request;
DROP TABLE IF EXISTS kb.doc_review_reports;
