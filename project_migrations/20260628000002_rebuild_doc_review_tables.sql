-- +goose Up
-- Rebuild all doc-review tables to replace the review_run_id TEXT soft-link with a
-- proper kb.doc_review_runs table and integer run_id FK (ADR 2026062804).
-- Clean-slate migration: all existing doc-review data is dropped.

DROP TABLE IF EXISTS kb.doc_review_activities;
DROP TABLE IF EXISTS kb.doc_review_status;
DROP TABLE IF EXISTS kb.doc_review_reports;
DROP TABLE IF EXISTS kb.doc_review_findings;
DROP TABLE IF EXISTS kb.doc_review_runs;
DROP TABLE IF EXISTS kb.doc_review_requests;

-- ── kb.doc_review_requests ────────────────────────────────────────────────────
-- One row per user submission. Captures intent and configuration.
-- Timing and execution fields have moved to kb.doc_review_runs.
CREATE TABLE IF NOT EXISTS kb.doc_review_requests (
    id              BIGSERIAL       PRIMARY KEY,
    input_record_id BIGINT          NOT NULL,
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
    create_time     TIMESTAMPTZ     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_doc_review_requests_record ON kb.doc_review_requests (input_record_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_requests_status ON kb.doc_review_requests (status);

-- ── kb.doc_review_runs ───────────────────────────────────────────────────────
-- One row per execution of a review request. A request may have multiple runs:
-- re-runs after failure, additional-aspect runs, or incremental runs.
CREATE TABLE IF NOT EXISTS kb.doc_review_runs (
    id              BIGSERIAL    PRIMARY KEY,
    request_id      BIGINT       NOT NULL,
    input_record_id BIGINT       NOT NULL,
    run_number      INT          NOT NULL DEFAULT 1,
    aspects         JSONB        NOT NULL,
    model_overrides JSONB,
    notes           TEXT,
    status          TEXT         NOT NULL DEFAULT 'pending',
    created_by      TEXT,
    create_time     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    start_time      TIMESTAMPTZ,
    end_time        TIMESTAMPTZ,
    error_message   TEXT
);

CREATE INDEX IF NOT EXISTS idx_doc_review_runs_request ON kb.doc_review_runs (request_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_runs_record  ON kb.doc_review_runs (input_record_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_runs_status  ON kb.doc_review_runs (request_id, status);

-- ── kb.doc_review_findings ───────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS kb.doc_review_findings (
    id              BIGSERIAL        PRIMARY KEY,
    input_record_id BIGINT           NOT NULL,
    run_id          BIGINT           NOT NULL,
    pass            TEXT             NOT NULL,
    aspect          TEXT             NOT NULL,
    severity        TEXT             NOT NULL,
    finding_type    TEXT             NOT NULL,
    title           TEXT             NOT NULL,
    description     TEXT             NOT NULL,
    evidence        TEXT,
    location        TEXT,
    reference_doc   JSONB,
    suggestion      TEXT,
    confidence      DOUBLE PRECISION,
    metadata        JSONB,
    reviewed_by     TEXT,
    review_status   TEXT             NOT NULL DEFAULT 'pending',
    create_time     TIMESTAMPTZ      NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_doc_review_findings_record   ON kb.doc_review_findings (input_record_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_findings_run      ON kb.doc_review_findings (run_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_findings_pass     ON kb.doc_review_findings (input_record_id, pass);
CREATE INDEX IF NOT EXISTS idx_doc_review_findings_aspect   ON kb.doc_review_findings (input_record_id, aspect);
CREATE INDEX IF NOT EXISTS idx_doc_review_findings_severity ON kb.doc_review_findings (input_record_id, severity);

-- ── kb.doc_review_reports ────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS kb.doc_review_reports (
    id                BIGSERIAL    PRIMARY KEY,
    request_id        BIGINT       NOT NULL,
    input_record_id   BIGINT       NOT NULL,
    run_id            BIGINT       NOT NULL,
    report_json       JSONB        NOT NULL,
    report_markdown   TEXT         NOT NULL,
    executive_summary TEXT         NOT NULL,
    total_findings    INT          NOT NULL,
    high_count        INT          NOT NULL DEFAULT 0,
    medium_count      INT          NOT NULL DEFAULT 0,
    low_count         INT          NOT NULL DEFAULT 0,
    overall_assessment TEXT        NOT NULL,
    create_time       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_doc_review_reports_request ON kb.doc_review_reports (request_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_reports_run     ON kb.doc_review_reports (run_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_reports_record  ON kb.doc_review_reports (input_record_id);

-- ── kb.doc_review_status ─────────────────────────────────────────────────────
-- Per-aspect progress tracking. One row per (run_id, aspect).
CREATE TABLE IF NOT EXISTS kb.doc_review_status (
    id              BIGSERIAL        PRIMARY KEY,
    request_id      BIGINT           NOT NULL,
    input_record_id BIGINT           NOT NULL,
    run_id          BIGINT           NOT NULL,
    aspect          TEXT             NOT NULL,
    pass            TEXT,
    status          TEXT             NOT NULL DEFAULT 'pending',
    progress        DOUBLE PRECISION NOT NULL DEFAULT 0,
    finding_count   INT              NOT NULL DEFAULT 0,
    error_message   TEXT,
    start_time      TIMESTAMPTZ,
    end_time        TIMESTAMPTZ,
    create_time     TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    modify_time     TIMESTAMPTZ      NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, aspect)
);

CREATE INDEX IF NOT EXISTS idx_doc_review_status_request ON kb.doc_review_status (request_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_status_run     ON kb.doc_review_status (run_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_status_active  ON kb.doc_review_status (run_id)
    WHERE status NOT IN ('success', 'failed');

-- ── kb.doc_review_activities ─────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS kb.doc_review_activities (
    id              BIGSERIAL    PRIMARY KEY,
    activity_type   TEXT         NOT NULL,
    input_record_id BIGINT       NOT NULL,
    run_id          BIGINT,
    report_id       BIGINT,
    finding_id      BIGINT,
    page_number     INT,
    line_number     INT,
    location        TEXT,
    old_content     TEXT,
    new_content     TEXT,
    detail          JSONB,
    actor           TEXT,
    create_time     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_doc_review_activities_record ON kb.doc_review_activities (input_record_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_activities_run    ON kb.doc_review_activities (run_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_activities_report ON kb.doc_review_activities (report_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_activities_type   ON kb.doc_review_activities (activity_type);

-- +goose Down
DROP TABLE IF EXISTS kb.doc_review_activities;
DROP TABLE IF EXISTS kb.doc_review_status;
DROP TABLE IF EXISTS kb.doc_review_reports;
DROP TABLE IF EXISTS kb.doc_review_findings;
DROP TABLE IF EXISTS kb.doc_review_runs;
DROP TABLE IF EXISTS kb.doc_review_requests;
