-- +goose Up
-- Copy column types and checks, without live-table defaults, sequences or FKs.
-- Archives survive deletion of their original run or input document.
CREATE TABLE IF NOT EXISTS testbed.generated_questions_archive (
    LIKE testbed.generated_questions INCLUDING CONSTRAINTS,
    archived_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    archived_by TEXT NOT NULL,
    PRIMARY KEY (id)
);

-- +goose Down
-- Restore the questions before removing the archive so rollback retains data.
INSERT INTO testbed.generated_questions
    (id, run_id, input_record_id, hop_type, question, answer, reasoning_steps,
     step_line_ids, evidence, metric_id, metric_snapshot, set_no, lang,
     source_sha256, purpose, prompt_name, model_name, created_at)
SELECT id, run_id, input_record_id, hop_type, question, answer, reasoning_steps,
       step_line_ids, evidence, metric_id, metric_snapshot, set_no, lang,
       source_sha256, purpose, prompt_name, model_name, created_at
FROM testbed.generated_questions_archive;
DROP TABLE IF EXISTS testbed.generated_questions_archive;
