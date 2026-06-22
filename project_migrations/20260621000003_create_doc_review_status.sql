-- +goose Up
-- DR15 — per-aspect review status. One row per (review_run_id, aspect), created at
-- request-accept time. The live job monitor lists every request that still has at
-- least one row with status NOT IN ('success','failed').
CREATE TABLE IF NOT EXISTS kb.doc_review_status (
    id              BIGSERIAL    PRIMARY KEY,
    request_id      BIGINT       NOT NULL,                    -- kb.doc_review_requests.id
    input_record_id BIGINT       NOT NULL,                    -- the document under review
    review_run_id   TEXT         NOT NULL,                    -- assigned at accept; matches requests + findings
    aspect          TEXT         NOT NULL,                    -- one row per reviewed aspect
    pass            TEXT,                                     -- "P1".."P6" (denormalized for grouping)
    status          TEXT         NOT NULL DEFAULT 'pending',  -- pending | running | success | failed
    finding_count   INT          NOT NULL DEFAULT 0,
    error_message   TEXT,
    start_time      TIMESTAMPTZ,
    end_time        TIMESTAMPTZ,
    create_time     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    modify_time     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (review_run_id, aspect)
);

CREATE INDEX IF NOT EXISTS idx_doc_review_status_request ON kb.doc_review_status (request_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_status_run     ON kb.doc_review_status (review_run_id);
-- Partial index accelerates the "active jobs" query (rows not yet finished).
CREATE INDEX IF NOT EXISTS idx_doc_review_status_active  ON kb.doc_review_status (request_id)
    WHERE status NOT IN ('success', 'failed');

-- +goose Down
DROP INDEX IF EXISTS idx_doc_review_status_active;
DROP INDEX IF EXISTS idx_doc_review_status_run;
DROP INDEX IF EXISTS idx_doc_review_status_request;
DROP TABLE IF EXISTS kb.doc_review_status;
