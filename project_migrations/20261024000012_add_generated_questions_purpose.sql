-- +goose Up
ALTER TABLE testbed.generated_questions ADD COLUMN IF NOT EXISTS purpose TEXT;

UPDATE testbed.generated_questions AS q
SET purpose = r.purpose
FROM testbed.generated_question_runs AS r
WHERE q.run_id = r.id AND q.purpose IS NULL;

ALTER TABLE testbed.generated_questions ALTER COLUMN purpose SET NOT NULL;

-- +goose Down
ALTER TABLE testbed.generated_questions DROP COLUMN IF EXISTS purpose;
