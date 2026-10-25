-- +goose Up
ALTER TABLE testbed.generated_question_runs
    DROP CONSTRAINT IF EXISTS generated_question_runs_one_hop_count_check,
    DROP CONSTRAINT IF EXISTS generated_question_runs_two_hop_count_check,
    DROP CONSTRAINT IF EXISTS generated_question_runs_multi_hop_count_check;
ALTER TABLE testbed.generated_question_runs
    ADD CONSTRAINT generated_question_runs_one_hop_count_check CHECK (one_hop_count BETWEEN 0 AND 1000),
    ADD CONSTRAINT generated_question_runs_two_hop_count_check CHECK (two_hop_count BETWEEN 0 AND 1000),
    ADD CONSTRAINT generated_question_runs_multi_hop_count_check CHECK (multi_hop_count BETWEEN 0 AND 1000);

-- +goose Down
-- Fails transactionally if a larger run exists; never truncate or delete run history.
ALTER TABLE testbed.generated_question_runs
    DROP CONSTRAINT IF EXISTS generated_question_runs_one_hop_count_check,
    DROP CONSTRAINT IF EXISTS generated_question_runs_two_hop_count_check,
    DROP CONSTRAINT IF EXISTS generated_question_runs_multi_hop_count_check;
ALTER TABLE testbed.generated_question_runs
    ADD CONSTRAINT generated_question_runs_one_hop_count_check CHECK (one_hop_count BETWEEN 0 AND 50),
    ADD CONSTRAINT generated_question_runs_two_hop_count_check CHECK (two_hop_count BETWEEN 0 AND 50),
    ADD CONSTRAINT generated_question_runs_multi_hop_count_check CHECK (multi_hop_count BETWEEN 0 AND 50);
