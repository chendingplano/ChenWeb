-- +goose Up
-- Gold set for the Compliance Review reviewer (openspec compliance-reviewer-phase2), written by the
-- compliance-gold command and read by compliance-score. One run labels the documents listed in
-- doc-review.local.toml against one evaluation snapshot. Labels come from two independent model
-- annotators and an adjudicator (profiles recorded on the run), so every score is LLM-adjudicated.
CREATE TABLE IF NOT EXISTS testbed.compliance_gold_runs (
    id                BIGSERIAL   PRIMARY KEY,
    run_key           TEXT        NOT NULL UNIQUE CHECK (run_key ~ '^[0-9]{8}_[0-9]{6}$'),
    snapshot_id       BIGINT      NOT NULL REFERENCES testbed.compliance_eval_snapshots(id) ON DELETE RESTRICT,
    rules_file        TEXT        NOT NULL,
    rules_sha256      TEXT        NOT NULL CHECK (rules_sha256 ~ '^[0-9a-f]{64}$'),
    annotator_1       TEXT        NOT NULL,
    annotator_2       TEXT        NOT NULL,
    adjudicator       TEXT        NOT NULL,
    pool_limit        INTEGER     NOT NULL CHECK (pool_limit > 0),
    organization      JSONB       NOT NULL CHECK (jsonb_typeof(organization) = 'object'),
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (annotator_1 <> annotator_2)
);

CREATE TABLE IF NOT EXISTS testbed.compliance_gold_activities (
    id              BIGSERIAL   PRIMARY KEY,
    run_id          BIGINT      NOT NULL REFERENCES testbed.compliance_gold_runs(id) ON DELETE CASCADE,
    input_record_id BIGINT      NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    covered         BOOLEAN     NOT NULL,   -- listed under covered-docs (true) or uncovered-docs (false)
    activity        TEXT        NOT NULL,
    object          TEXT        NOT NULL DEFAULT '',
    line_start      INTEGER     NOT NULL,
    line_end        INTEGER     NOT NULL CHECK (line_end >= line_start),
    document_level  BOOLEAN     NOT NULL DEFAULT FALSE,
    agreement       TEXT        NOT NULL CHECK (agreement IN ('both', 'adjudicated_kept')),
    rationale       TEXT        NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_compliance_gold_activities_run ON testbed.compliance_gold_activities (run_id, input_record_id);

CREATE TABLE IF NOT EXISTS testbed.compliance_gold_judgements (
    id                BIGSERIAL   PRIMARY KEY,
    activity_id       BIGINT      NOT NULL REFERENCES testbed.compliance_gold_activities(id) ON DELETE CASCADE,
    artifact_type     TEXT        NOT NULL,
    artifact_id       TEXT        NOT NULL,
    candidate_record  BIGINT      NOT NULL,
    cosine            DOUBLE PRECISION,
    label_1           TEXT        NOT NULL CHECK (label_1 IN ('governs', 'topical', 'unrelated')),
    reason_1          TEXT        NOT NULL DEFAULT '',
    label_2           TEXT        NOT NULL CHECK (label_2 IN ('governs', 'topical', 'unrelated')),
    reason_2          TEXT        NOT NULL DEFAULT '',
    final_label       TEXT        NOT NULL CHECK (final_label IN ('governs', 'topical', 'unrelated', 'ambiguous')),
    adjudicated       BOOLEAN     NOT NULL,
    rationale         TEXT        NOT NULL DEFAULT '',
    UNIQUE (activity_id, artifact_type, artifact_id)
);

-- +goose Down
DROP TABLE IF EXISTS testbed.compliance_gold_judgements;
DROP TABLE IF EXISTS testbed.compliance_gold_activities;
DROP TABLE IF EXISTS testbed.compliance_gold_runs;
