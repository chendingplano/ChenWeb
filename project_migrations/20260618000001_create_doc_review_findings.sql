-- +goose Up
CREATE TABLE IF NOT EXISTS kb.doc_review_findings (
    id              BIGSERIAL       PRIMARY KEY,
    input_record_id BIGINT          NOT NULL,
    review_run_id   TEXT            NOT NULL,
    pass            TEXT            NOT NULL,
    aspect          TEXT            NOT NULL,
    severity        TEXT            NOT NULL,
    finding_type    TEXT            NOT NULL,
    title           TEXT            NOT NULL,
    description     TEXT            NOT NULL,
    evidence        TEXT,
    location        JSONB,
    reference_doc   JSONB,
    suggestion      TEXT,
    confidence      DOUBLE PRECISION,
    metadata        JSONB,
    reviewed_by     TEXT,
    review_status   TEXT            NOT NULL DEFAULT 'pending',
    create_time     TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_doc_review_findings_record
    ON kb.doc_review_findings (input_record_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_findings_pass
    ON kb.doc_review_findings (input_record_id, pass);
CREATE INDEX IF NOT EXISTS idx_doc_review_findings_aspect
    ON kb.doc_review_findings (input_record_id, aspect);
CREATE INDEX IF NOT EXISTS idx_doc_review_findings_severity
    ON kb.doc_review_findings (input_record_id, severity);
CREATE INDEX IF NOT EXISTS idx_doc_review_findings_run
    ON kb.doc_review_findings (input_record_id, review_run_id);

-- +goose Down
DROP INDEX IF EXISTS idx_doc_review_findings_run;
DROP INDEX IF EXISTS idx_doc_review_findings_severity;
DROP INDEX IF EXISTS idx_doc_review_findings_aspect;
DROP INDEX IF EXISTS idx_doc_review_findings_pass;
DROP INDEX IF EXISTS idx_doc_review_findings_record;
DROP TABLE IF EXISTS kb.doc_review_findings;
