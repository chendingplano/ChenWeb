-- +goose Up
-- Each retired duplicate remains in assertion_evidence as soft-deleted
-- history. This audit table records the deterministic survivor and the exact
-- occurrence whose current-support cardinality was repaired.
CREATE TABLE IF NOT EXISTS kb.metric_support_cleanup_audit (
    id                  BIGSERIAL PRIMARY KEY,
    artifact_type       TEXT NOT NULL,
    artifact_id         TEXT NOT NULL,
    input_record_id     BIGINT,
    retained_evidence_id BIGINT NOT NULL REFERENCES kb.assertion_evidence(id),
    retired_evidence_id  BIGINT NOT NULL REFERENCES kb.assertion_evidence(id),
    cleanup_reason      TEXT NOT NULL,
    create_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (retired_evidence_id),
    CHECK (retained_evidence_id <> retired_evidence_id)
);

CREATE INDEX IF NOT EXISTS idx_metric_support_cleanup_occurrence
    ON kb.metric_support_cleanup_audit (artifact_type, artifact_id, input_record_id);

-- +goose Down
DROP TABLE IF EXISTS kb.metric_support_cleanup_audit;
