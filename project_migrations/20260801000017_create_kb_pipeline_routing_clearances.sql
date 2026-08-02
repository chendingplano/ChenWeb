-- +goose Up
CREATE TABLE IF NOT EXISTS kb.pipeline_routing_clearances (
    id BIGSERIAL PRIMARY KEY,
    policy_id BIGINT NOT NULL REFERENCES kb.pipeline_policies(id) ON DELETE RESTRICT,
    policy_version INT NOT NULL,
    document_kind TEXT NOT NULL,
    manifest_checksum TEXT NOT NULL,
    baseline_run_id UUID NOT NULL REFERENCES kb.benchmark_runs(id) ON DELETE RESTRICT,
    routed_run_id UUID NOT NULL REFERENCES kb.benchmark_runs(id) ON DELETE RESTRICT,
    repetitions INT NOT NULL CHECK (repetitions > 0),
    paired_case_count INT NOT NULL CHECK (paired_case_count >= 3),
    gold_positive_denominator INT NOT NULL CHECK (gold_positive_denominator > 0),
    processor_failure_count INT NOT NULL DEFAULT 0 CHECK (processor_failure_count >= 0),
    infrastructure_failure_count INT NOT NULL DEFAULT 0 CHECK (infrastructure_failure_count >= 0),
    scorer_failure_count INT NOT NULL DEFAULT 0 CHECK (scorer_failure_count >= 0),
    baseline_recall_numerator INT NOT NULL,
    baseline_recall_denominator INT NOT NULL CHECK (baseline_recall_denominator > 0),
    routed_recall_numerator INT NOT NULL,
    routed_recall_denominator INT NOT NULL CHECK (routed_recall_denominator > 0),
    baseline_precision DOUBLE PRECISION,
    routed_precision DOUBLE PRECISION,
    cost_yield JSONB NOT NULL DEFAULT '{}'::jsonb,
    decision_traces JSONB NOT NULL DEFAULT '[]'::jsonb,
    approved_by TEXT NOT NULL,
    rationale TEXT NOT NULL,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS kb.pipeline_routing_clearance_coverage (
    id BIGSERIAL PRIMARY KEY,
    clearance_id BIGINT NOT NULL REFERENCES kb.pipeline_routing_clearances(id) ON DELETE RESTRICT,
    policy_id BIGINT NOT NULL REFERENCES kb.pipeline_policies(id) ON DELETE RESTRICT,
    policy_version INT NOT NULL,
    subject_kind TEXT NOT NULL CHECK (subject_kind IN ('processor_rule', 'conditional_binding')),
    subject_id BIGINT NOT NULL,
    subject_checksum TEXT NOT NULL,
    document_kind TEXT NOT NULL,
    net_plan_delta_checksum TEXT NOT NULL,
    superseded_clearance_id BIGINT REFERENCES kb.pipeline_routing_clearances(id) ON DELETE RESTRICT,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (policy_id, policy_version, subject_kind, subject_id, document_kind, clearance_id)
);

CREATE TABLE IF NOT EXISTS kb.pipeline_routing_clearance_revocations (
    id BIGSERIAL PRIMARY KEY,
    clearance_id BIGINT NOT NULL REFERENCES kb.pipeline_routing_clearances(id) ON DELETE RESTRICT,
    revoked_by TEXT NOT NULL,
    reason TEXT NOT NULL,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pipeline_routing_clearance_coverage_subject
    ON kb.pipeline_routing_clearance_coverage
    (policy_id, policy_version, subject_kind, subject_id, document_kind, subject_checksum, net_plan_delta_checksum);

CREATE INDEX IF NOT EXISTS idx_pipeline_routing_clearance_revocations_clearance
    ON kb.pipeline_routing_clearance_revocations (clearance_id);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_pipeline_routing_clearance_revocations_clearance;
DROP INDEX IF EXISTS kb.idx_pipeline_routing_clearance_coverage_subject;
DROP TABLE IF EXISTS kb.pipeline_routing_clearance_revocations;
DROP TABLE IF EXISTS kb.pipeline_routing_clearance_coverage;
DROP TABLE IF EXISTS kb.pipeline_routing_clearances;
