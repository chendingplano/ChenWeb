-- +goose Up
-- Evaluation snapshots for the Compliance Review gold set (openspec compliance-reviewer-phase2).
-- A snapshot does not copy the searched corpus (that stays in kb.search_artifacts_*); it records
-- which records a gold run may rely on and the state they were in, so a run can refuse to start
-- when a member's file (md5), chunk-index generation or index counts changed since the freeze.
CREATE TABLE IF NOT EXISTS testbed.compliance_eval_snapshots (
    id              BIGSERIAL   PRIMARY KEY,
    name            TEXT        NOT NULL UNIQUE,
    embedding_model TEXT        NOT NULL,
    notes           TEXT        NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS testbed.compliance_eval_snapshot_records (
    snapshot_id     BIGINT      NOT NULL REFERENCES testbed.compliance_eval_snapshots(id) ON DELETE CASCADE,
    input_record_id BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    md5             TEXT        NOT NULL DEFAULT '',
    generation      BIGINT      NOT NULL,
    chunk_count     INTEGER     NOT NULL CHECK (chunk_count >= 0),
    provision_count INTEGER     NOT NULL CHECK (provision_count >= 0),
    PRIMARY KEY (snapshot_id, input_record_id)
);

-- +goose Down
DROP TABLE IF EXISTS testbed.compliance_eval_snapshot_records;
DROP TABLE IF EXISTS testbed.compliance_eval_snapshots;
