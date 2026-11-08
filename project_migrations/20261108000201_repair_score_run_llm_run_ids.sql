-- +goose Up
-- 20261108000001 was applied on some databases before its score-run columns were added
-- to the file, so those databases never got them. Without the column, taskLLMExecution's
-- UPDATE fails and the run keeps no llm_run_id. Re-add every column that migration owns;
-- on databases that ran its final text this is a no-op.
ALTER TABLE kb.metric_score_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;
ALTER TABLE kb.reference_score_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;
ALTER TABLE testbed.provision_score_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;
ALTER TABLE kb.summary_score_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;
ALTER TABLE testbed.generated_question_runs ADD COLUMN IF NOT EXISTS llm_run_id BIGINT;

-- +goose Down
-- Nothing to undo: the columns belong to 20261108000001, whose Down drops them.
