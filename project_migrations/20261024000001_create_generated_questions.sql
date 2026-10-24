-- +goose Up
CREATE SCHEMA IF NOT EXISTS testbed;

CREATE TABLE IF NOT EXISTS testbed.generated_question_runs (
    id BIGSERIAL PRIMARY KEY,
    input_record_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    purpose TEXT NOT NULL CHECK (btrim(purpose) <> ''),
    description TEXT NOT NULL DEFAULT '',
    one_hop_count INTEGER NOT NULL DEFAULT 10 CHECK (one_hop_count BETWEEN 0 AND 50),
    two_hop_count INTEGER NOT NULL DEFAULT 10 CHECK (two_hop_count BETWEEN 0 AND 50),
    multi_hop_count INTEGER NOT NULL DEFAULT 10 CHECK (multi_hop_count BETWEEN 0 AND 50),
    lang TEXT NOT NULL CHECK (lang IN ('en', 'zh-cn')),
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'done', 'failed')),
    error_msg TEXT NOT NULL DEFAULT '',
    model_ref TEXT NOT NULL,
    model_name TEXT NOT NULL DEFAULT '',
    prompt_name TEXT NOT NULL,
    source_sha256 TEXT NOT NULL DEFAULT '',
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    CHECK (one_hop_count + two_hop_count + multi_hop_count > 0),
    UNIQUE (id, input_record_id)
);
CREATE INDEX IF NOT EXISTS idx_generated_question_runs_input
    ON testbed.generated_question_runs (input_record_id, id DESC);

CREATE TABLE IF NOT EXISTS testbed.generated_questions (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL,
    input_record_id BIGINT NOT NULL,
    hop_type TEXT NOT NULL CHECK (hop_type IN ('one_hop', 'two_hop', 'multi_hop')),
    question TEXT NOT NULL CHECK (btrim(question) <> ''),
    answer TEXT NOT NULL CHECK (btrim(answer) <> ''),
    reasoning_steps JSONB NOT NULL CHECK (jsonb_typeof(reasoning_steps) = 'array'),
    step_line_ids JSONB NOT NULL CHECK (jsonb_typeof(step_line_ids) = 'array'),
    evidence JSONB NOT NULL CHECK (jsonb_typeof(evidence) = 'array'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, question),
    FOREIGN KEY (run_id, input_record_id)
        REFERENCES testbed.generated_question_runs (id, input_record_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_generated_questions_input
    ON testbed.generated_questions (input_record_id, run_id);

-- +goose Down
DROP TABLE IF EXISTS testbed.generated_questions;
DROP TABLE IF EXISTS testbed.generated_question_runs;
