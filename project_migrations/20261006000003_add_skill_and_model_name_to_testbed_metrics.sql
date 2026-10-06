-- +goose Up
-- Provenance of each benchmark row: the skill that produced it and the LLM that ran the skill.
ALTER TABLE testbed.metrics ADD COLUMN IF NOT EXISTS skill_name TEXT;
ALTER TABLE testbed.metrics ADD COLUMN IF NOT EXISTS model_name TEXT;
-- All rows saved so far came from extract-metrics-benchmark.
UPDATE testbed.metrics SET skill_name = 'extract-metrics-benchmark' WHERE skill_name IS NULL;
-- Run 22aa157c was produced in a Codex session (rollout 2026-10-06T06-22-06) running gpt-6.1-sol.
UPDATE testbed.metrics SET model_name = 'gpt-6.1-sol'
 WHERE model_name IS NULL AND benchmark_run_id = '22aa157c-d712-46be-abc8-d7f195cff483';
UPDATE testbed.metrics SET model_name = 'unknown' WHERE model_name IS NULL;
ALTER TABLE testbed.metrics ALTER COLUMN skill_name SET NOT NULL;
ALTER TABLE testbed.metrics ALTER COLUMN model_name SET NOT NULL;

-- +goose Down
ALTER TABLE testbed.metrics DROP COLUMN IF EXISTS model_name;
ALTER TABLE testbed.metrics DROP COLUMN IF EXISTS skill_name;
