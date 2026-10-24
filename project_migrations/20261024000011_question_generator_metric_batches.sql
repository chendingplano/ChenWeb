-- +goose Up
ALTER TABLE testbed.generated_question_runs
    ALTER COLUMN input_record_id DROP NOT NULL,
    DROP CONSTRAINT IF EXISTS generated_question_runs_lang_check,
    ADD COLUMN IF NOT EXISTS total_one_hop_count INTEGER NOT NULL DEFAULT 0 CHECK (total_one_hop_count >= 0),
    ADD COLUMN IF NOT EXISTS total_two_hop_count INTEGER NOT NULL DEFAULT 0 CHECK (total_two_hop_count >= 0),
    ADD COLUMN IF NOT EXISTS total_multi_hop_count INTEGER NOT NULL DEFAULT 0 CHECK (total_multi_hop_count >= 0),
    ADD COLUMN IF NOT EXISTS generated_one_hop_count INTEGER NOT NULL DEFAULT 0 CHECK (generated_one_hop_count >= 0),
    ADD COLUMN IF NOT EXISTS generated_two_hop_count INTEGER NOT NULL DEFAULT 0 CHECK (generated_two_hop_count >= 0),
    ADD COLUMN IF NOT EXISTS generated_multi_hop_count INTEGER NOT NULL DEFAULT 0 CHECK (generated_multi_hop_count >= 0),
    ADD COLUMN IF NOT EXISTS documents_processed INTEGER NOT NULL DEFAULT 0 CHECK (documents_processed >= 0),
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '10 minutes');
ALTER TABLE testbed.generated_question_runs
    ADD CONSTRAINT generated_question_runs_lang_check CHECK (lang IN ('en', 'zh-cn', 'mixed'));

UPDATE testbed.generated_question_runs SET
    total_one_hop_count = one_hop_count,
    total_two_hop_count = two_hop_count,
    total_multi_hop_count = multi_hop_count,
    expires_at = created_at + INTERVAL '10 minutes';
UPDATE testbed.generated_question_runs r SET
    generated_one_hop_count = q.one_hop,
    generated_two_hop_count = q.two_hop,
    generated_multi_hop_count = q.multi_hop,
    documents_processed = q.documents
FROM (
    SELECT run_id,
        COUNT(*) FILTER (WHERE hop_type = 'one_hop')::integer AS one_hop,
        COUNT(*) FILTER (WHERE hop_type = 'two_hop')::integer AS two_hop,
        COUNT(*) FILTER (WHERE hop_type = 'multi_hop')::integer AS multi_hop,
        COUNT(DISTINCT input_record_id)::integer AS documents
    FROM testbed.generated_questions GROUP BY run_id
) q WHERE r.id = q.run_id;

ALTER TABLE testbed.generated_questions
    DROP CONSTRAINT IF EXISTS generated_questions_run_id_input_record_id_fkey,
    ADD COLUMN IF NOT EXISTS metric_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS metric_snapshot JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS set_no INTEGER NOT NULL DEFAULT 1 CHECK (set_no > 0),
    ADD COLUMN IF NOT EXISTS lang TEXT NOT NULL DEFAULT 'en',
    ADD COLUMN IF NOT EXISTS source_sha256 TEXT NOT NULL DEFAULT '';
ALTER TABLE testbed.generated_questions
    ADD CONSTRAINT generated_questions_run_id_fkey FOREIGN KEY (run_id)
        REFERENCES testbed.generated_question_runs(id) ON DELETE CASCADE,
    ADD CONSTRAINT generated_questions_input_record_id_fkey FOREIGN KEY (input_record_id)
        REFERENCES kb.inputs(id) ON DELETE CASCADE,
    ADD CONSTRAINT generated_questions_lang_check CHECK (lang IN ('en', 'zh-cn'));
UPDATE testbed.generated_questions q SET lang = r.lang, source_sha256 = r.source_sha256
FROM testbed.generated_question_runs r WHERE r.id = q.run_id;

-- +goose Down
-- Refuse before changing any schema or data: legacy runs cannot represent batches.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM testbed.generated_question_runs WHERE input_record_id IS NULL) THEN
        RAISE EXCEPTION 'Cannot roll back question generator metric batches while batch runs exist';
    END IF;
END
$$;
-- +goose StatementEnd

ALTER TABLE testbed.generated_questions
    DROP CONSTRAINT IF EXISTS generated_questions_lang_check,
    DROP CONSTRAINT IF EXISTS generated_questions_input_record_id_fkey,
    DROP CONSTRAINT IF EXISTS generated_questions_run_id_fkey,
    DROP COLUMN IF EXISTS source_sha256,
    DROP COLUMN IF EXISTS lang,
    DROP COLUMN IF EXISTS set_no,
    DROP COLUMN IF EXISTS metric_snapshot,
    DROP COLUMN IF EXISTS metric_id;
ALTER TABLE testbed.generated_questions
    ADD CONSTRAINT generated_questions_run_id_input_record_id_fkey
        FOREIGN KEY (run_id, input_record_id)
        REFERENCES testbed.generated_question_runs(id, input_record_id) ON DELETE CASCADE;
ALTER TABLE testbed.generated_question_runs
    DROP CONSTRAINT IF EXISTS generated_question_runs_lang_check,
    DROP COLUMN IF EXISTS expires_at,
    DROP COLUMN IF EXISTS documents_processed,
    DROP COLUMN IF EXISTS generated_multi_hop_count,
    DROP COLUMN IF EXISTS generated_two_hop_count,
    DROP COLUMN IF EXISTS generated_one_hop_count,
    DROP COLUMN IF EXISTS total_multi_hop_count,
    DROP COLUMN IF EXISTS total_two_hop_count,
    DROP COLUMN IF EXISTS total_one_hop_count,
    ALTER COLUMN input_record_id SET NOT NULL;
UPDATE testbed.generated_question_runs SET lang = 'en' WHERE lang = 'mixed';
ALTER TABLE testbed.generated_question_runs
    ADD CONSTRAINT generated_question_runs_lang_check CHECK (lang IN ('en', 'zh-cn'));
