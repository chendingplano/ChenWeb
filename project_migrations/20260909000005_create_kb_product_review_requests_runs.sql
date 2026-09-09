-- +goose Up
-- Product Metric Reviewer (openspec change product-metric-reviewer), capability
-- product-metric-review-runs. Request/run split mirrors doc-review
-- (kb.doc_review_requests / _runs, ADR 2026062804): a request holds intent
-- (profile + pinned version, artifact types, filters), a run holds one
-- execution. Re-run creates a new run with the next run_number under the same
-- request, so "what became known this month" is a diff of two runs.
--
-- Deviation from doc-review, deliberate: one report per run, so the report is a
-- column pair (report_json + report_md) on the run row, not a separate table.

CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.product_review_requests (
    id               BIGSERIAL    PRIMARY KEY,
    tenant_id        VARCHAR(128) NOT NULL DEFAULT '-',
    profile_id       BIGINT       NOT NULL REFERENCES kb.product_profiles(id) ON DELETE CASCADE,
    profile_version  INT          NOT NULL,
    artifact_types   JSONB        NOT NULL DEFAULT '["metric"]'::jsonb,
    filters          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    notes            TEXT         NOT NULL DEFAULT '',
    requester        TEXT         NOT NULL DEFAULT '',
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_product_review_requests_profile
    ON kb.product_review_requests (profile_id);
CREATE INDEX IF NOT EXISTS idx_kb_product_review_requests_tenant
    ON kb.product_review_requests (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS kb.product_review_runs (
    id                     BIGSERIAL   PRIMARY KEY,
    request_id             BIGINT      NOT NULL REFERENCES kb.product_review_requests(id) ON DELETE CASCADE,
    run_number             INT         NOT NULL,
    status                 TEXT        NOT NULL DEFAULT 'pending'
                                       CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    started_at             TIMESTAMPTZ,
    finished_at            TIMESTAMPTZ,
    result_count           INT         NOT NULL DEFAULT 0,
    attributed_count       INT         NOT NULL DEFAULT 0,
    document_scope_count   INT         NOT NULL DEFAULT 0,
    scoped_document_count  INT         NOT NULL DEFAULT 0,
    truncated_count        INT         NOT NULL DEFAULT 0,
    report_json            JSONB       NOT NULL DEFAULT '{}'::jsonb,
    report_md              TEXT        NOT NULL DEFAULT '',
    error_message          TEXT        NOT NULL DEFAULT '',
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_kb_product_review_runs_request_number UNIQUE (request_id, run_number)
);

CREATE INDEX IF NOT EXISTS idx_kb_product_review_runs_request
    ON kb.product_review_runs (request_id, run_number DESC);
CREATE INDEX IF NOT EXISTS idx_kb_product_review_runs_status
    ON kb.product_review_runs (status);

-- +goose Down
DROP TABLE IF EXISTS kb.product_review_runs;
DROP TABLE IF EXISTS kb.product_review_requests;
