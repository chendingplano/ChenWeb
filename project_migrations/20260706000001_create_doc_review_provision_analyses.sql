-- +goose Up
-- Persists the mandatory per-match comparison analysis produced by the
-- provisions reviewer (prompt-review-provisions-v4), so the reasoning behind
-- "no conflict" conclusions is retained instead of being discarded when the
-- comparison does not rise to a finding.

CREATE TABLE IF NOT EXISTS kb.doc_review_provision_analyses (
    id                  BIGSERIAL    PRIMARY KEY,
    input_record_id     BIGINT       NOT NULL,
    run_id              BIGINT       NOT NULL,
    prov_id             TEXT         NOT NULL,
    related_artifact_id TEXT,
    related_record_id   BIGINT,
    relationship        TEXT         NOT NULL,
    summary             TEXT         NOT NULL,
    create_time         TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_doc_review_provision_analyses_record ON kb.doc_review_provision_analyses (input_record_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_provision_analyses_run    ON kb.doc_review_provision_analyses (run_id);
CREATE INDEX IF NOT EXISTS idx_doc_review_provision_analyses_prov   ON kb.doc_review_provision_analyses (prov_id);

-- +goose Down
DROP TABLE IF EXISTS kb.doc_review_provision_analyses;
