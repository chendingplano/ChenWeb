-- +goose Up
-- Sequence data is bootstrapped after both migration tracks at startup.
CREATE TABLE IF NOT EXISTS kb.llm_executions (
    execution_key TEXT PRIMARY KEY,
    llm_run_id BIGINT NOT NULL CHECK (llm_run_id >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
ALTER TABLE kb.doc_process_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;
ALTER TABLE kb.doc_review_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;
ALTER TABLE kb.metric_score_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;
ALTER TABLE kb.reference_score_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;
ALTER TABLE testbed.provision_score_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;
ALTER TABLE kb.summary_score_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;
ALTER TABLE testbed.generated_question_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;
COMMENT ON COLUMN kb.doc_process_runs.llm_run_id IS 'LLM usage correlation ID; 0 means allocation failed; NULL means legacy execution';
COMMENT ON COLUMN kb.doc_review_runs.llm_run_id IS 'LLM usage correlation ID; 0 means allocation failed; NULL means legacy execution';

-- +goose Down
-- Preserve shared.id_mgr history: deleting the sequence could reissue IDs.
ALTER TABLE testbed.generated_question_runs DROP COLUMN IF EXISTS llm_run_id;
ALTER TABLE kb.summary_score_runs DROP COLUMN IF EXISTS llm_run_id;
ALTER TABLE testbed.provision_score_runs DROP COLUMN IF EXISTS llm_run_id;
ALTER TABLE kb.reference_score_runs DROP COLUMN IF EXISTS llm_run_id;
ALTER TABLE kb.metric_score_runs DROP COLUMN IF EXISTS llm_run_id;
ALTER TABLE kb.doc_review_runs DROP COLUMN IF EXISTS llm_run_id;
ALTER TABLE kb.doc_process_runs DROP COLUMN IF EXISTS llm_run_id;
DROP TABLE IF EXISTS kb.llm_executions;
