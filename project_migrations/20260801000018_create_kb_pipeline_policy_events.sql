-- +goose Up
CREATE TABLE IF NOT EXISTS kb.pipeline_policy_events (
    id              BIGSERIAL    PRIMARY KEY,
    event_kind      TEXT         NOT NULL,
    policy_id       BIGINT       REFERENCES kb.pipeline_policies(id) ON DELETE RESTRICT,
    policy_version  INT,
    subject_kind    TEXT,
    subject_id      BIGINT,
    run_id          BIGINT,
    record_id       BIGINT,
    actor           TEXT,
    detail          JSONB        NOT NULL DEFAULT '{}'::jsonb,
    occurred_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pipeline_policy_events_kind_time
    ON kb.pipeline_policy_events (event_kind, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_pipeline_policy_events_policy
    ON kb.pipeline_policy_events (policy_id, policy_version);
CREATE INDEX IF NOT EXISTS idx_pipeline_policy_events_run
    ON kb.pipeline_policy_events (run_id);

COMMENT ON TABLE kb.pipeline_policy_events IS
    'Append-only P5 policy/routing audit log (ADR 2026072901 DR3/DR6, spec 2026080102 section 10). '
    'One row per authoring/activation/conflict/fallback/clearance/enforcement event. detail is '
    'content-safe (ids/checksums/booleans/processor names only, never document content).';

-- +goose Down
DROP INDEX IF EXISTS kb.idx_pipeline_policy_events_run;
DROP INDEX IF EXISTS kb.idx_pipeline_policy_events_policy;
DROP INDEX IF EXISTS kb.idx_pipeline_policy_events_kind_time;
DROP TABLE IF EXISTS kb.pipeline_policy_events;
