-- +goose Up
-- Compliance Review redesign (openspec compliance-reviewer-redesign): one row per failed try of a
-- three-try call (reviewer model, correction model, fallback model). fallback_model is empty when no
-- later try succeeded.
ALTER TABLE kb.failed_llm_calls ALTER COLUMN fallback_model DROP NOT NULL;
ALTER TABLE kb.failed_llm_calls ADD COLUMN IF NOT EXISTS attempt INTEGER;
ALTER TABLE kb.failed_llm_calls ADD COLUMN IF NOT EXISTS model TEXT;
ALTER TABLE kb.failed_llm_calls ADD COLUMN IF NOT EXISTS outcome TEXT;
ALTER TABLE kb.failed_llm_calls ADD COLUMN IF NOT EXISTS review_run_id BIGINT;
ALTER TABLE kb.failed_llm_calls ADD COLUMN IF NOT EXISTS aspect TEXT;

CREATE INDEX IF NOT EXISTS idx_failed_llm_calls_review_run
    ON kb.failed_llm_calls (review_run_id) WHERE review_run_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS kb.idx_failed_llm_calls_review_run;
ALTER TABLE kb.failed_llm_calls DROP COLUMN IF EXISTS aspect;
ALTER TABLE kb.failed_llm_calls DROP COLUMN IF EXISTS review_run_id;
ALTER TABLE kb.failed_llm_calls DROP COLUMN IF EXISTS outcome;
ALTER TABLE kb.failed_llm_calls DROP COLUMN IF EXISTS model;
ALTER TABLE kb.failed_llm_calls DROP COLUMN IF EXISTS attempt;
UPDATE kb.failed_llm_calls SET fallback_model = '' WHERE fallback_model IS NULL;
ALTER TABLE kb.failed_llm_calls ALTER COLUMN fallback_model SET NOT NULL;
