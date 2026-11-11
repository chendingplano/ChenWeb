-- +goose Up
-- Question targets of any search artifact type (openspec question-generator-artifact-types).
-- The archive was created with LIKE, so it does not follow the live table and gets the same columns.
ALTER TABLE testbed.generated_questions
    ADD COLUMN IF NOT EXISTS artifact_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS artifact_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS artifact_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE testbed.generated_questions_archive
    ADD COLUMN IF NOT EXISTS artifact_type TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS artifact_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS artifact_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE testbed.generated_questions SET artifact_type = 'metric', artifact_id = metric_id
WHERE metric_id <> '' AND artifact_id = '';
UPDATE testbed.generated_questions_archive SET artifact_type = 'metric', artifact_id = metric_id
WHERE metric_id <> '' AND artifact_id = '';

-- +goose Down
-- Refuse before changing anything: dropping the columns would orphan non-metric questions.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM testbed.generated_questions WHERE artifact_type NOT IN ('', 'metric'))
       OR EXISTS (SELECT 1 FROM testbed.generated_questions_archive WHERE artifact_type NOT IN ('', 'metric')) THEN
        RAISE EXCEPTION 'Cannot roll back question artifact targets while non-metric questions exist';
    END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE testbed.generated_questions_archive
    DROP COLUMN IF EXISTS artifact_snapshot,
    DROP COLUMN IF EXISTS artifact_id,
    DROP COLUMN IF EXISTS artifact_type;
ALTER TABLE testbed.generated_questions
    DROP COLUMN IF EXISTS artifact_snapshot,
    DROP COLUMN IF EXISTS artifact_id,
    DROP COLUMN IF EXISTS artifact_type;
