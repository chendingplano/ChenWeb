-- +goose Up
-- Independently reviewed candidates excluded or deduplicated by the benchmark skill.
-- Keep the complete candidate, including its rule, rationale and source references.
CREATE TABLE IF NOT EXISTS testbed.metrics_dropped (
    id BIGSERIAL PRIMARY KEY,
    input_record_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE RESTRICT,
    skill_name TEXT NOT NULL,
    skill_version TEXT NOT NULL,
    model_name TEXT NOT NULL,
    run_id TEXT NOT NULL,
    candidate_id TEXT NOT NULL,
    decision TEXT NOT NULL CHECK (decision IN ('excluded', 'duplicate')),
    drop_reason TEXT NOT NULL,
    candidate_data JSONB NOT NULL CHECK (jsonb_typeof(candidate_data) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (skill_name, skill_version, model_name, run_id, candidate_id),
    FOREIGN KEY (skill_name, skill_version, model_name, run_id)
        REFERENCES testbed.metric_benchmark_runs
            (skill_name, skill_version, model_name, benchmark_run_id) ON DELETE RESTRICT
);
CREATE INDEX IF NOT EXISTS idx_testbed_metrics_dropped_document_run
    ON testbed.metrics_dropped (input_record_id, skill_name, skill_version, model_name, run_id);
COMMENT ON COLUMN testbed.metrics_dropped.run_id IS
    'Same run ID as testbed.metrics.run_id; also match skill_name, skill_version and model_name.';

-- Existing benchmarks already retained their discarded candidates in the run audit ledger.
INSERT INTO testbed.metrics_dropped
    (input_record_id, skill_name, skill_version, model_name, run_id, candidate_id,
     decision, drop_reason, candidate_data, created_at)
SELECT r.input_record_id, r.skill_name, r.skill_version, r.model_name, r.benchmark_run_id,
       c->>'candidate_id', c->>'decision', COALESCE(NULLIF(c->>'rule_id', ''), c->>'decision'),
       c, r.created_at
  FROM testbed.metric_benchmark_runs r
 CROSS JOIN LATERAL jsonb_array_elements(r.candidates) c
 WHERE c->>'decision' IN ('excluded', 'duplicate')
ON CONFLICT DO NOTHING;

-- +goose Down
DROP INDEX IF EXISTS testbed.idx_testbed_metrics_dropped_document_run;
DROP TABLE IF EXISTS testbed.metrics_dropped;
