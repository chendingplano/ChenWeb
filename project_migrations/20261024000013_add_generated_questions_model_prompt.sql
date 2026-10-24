-- +goose Up
ALTER TABLE testbed.generated_questions
    ADD COLUMN IF NOT EXISTS prompt_name TEXT,
    ADD COLUMN IF NOT EXISTS model_name TEXT;

UPDATE testbed.generated_questions AS q
SET prompt_name = COALESCE(q.prompt_name, r.prompt_name),
    model_name = COALESCE(q.model_name, r.model_name)
FROM testbed.generated_question_runs AS r
WHERE q.run_id = r.id AND (q.prompt_name IS NULL OR q.model_name IS NULL);

ALTER TABLE testbed.generated_questions
    ALTER COLUMN prompt_name SET NOT NULL,
    ALTER COLUMN model_name SET NOT NULL;

-- +goose Down
ALTER TABLE testbed.generated_questions
    DROP COLUMN IF EXISTS model_name,
    DROP COLUMN IF EXISTS prompt_name;
