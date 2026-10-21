-- +goose Up
CREATE SCHEMA IF NOT EXISTS testbed;

CREATE TABLE IF NOT EXISTS testbed.summary_benchmark_runs (
    skill_name TEXT NOT NULL,
    skill_version TEXT NOT NULL,
    model_name TEXT NOT NULL,
    run_id TEXT NOT NULL CHECK (run_id ~ '^[0-9]{8}_[0-9]{6}$'),
    input_record_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    payload JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (skill_name, skill_version, model_name, run_id),
    UNIQUE (skill_name, skill_version, model_name, run_id, input_record_id)
);

CREATE TABLE IF NOT EXISTS testbed.summaries (
    id BIGSERIAL PRIMARY KEY,
    skill_name TEXT NOT NULL,
    skill_version TEXT NOT NULL,
    model_name TEXT NOT NULL,
    run_id TEXT NOT NULL,
    input_record_id BIGINT NOT NULL,
    summary_id TEXT NOT NULL,
    summary_level INTEGER NOT NULL CHECK (summary_level = 0),
    summary_seq_no INTEGER NOT NULL CHECK (summary_seq_no > 0),
    source_line_spans JSONB NOT NULL CHECK (jsonb_typeof(source_line_spans) = 'array'),
    overlap_lines JSONB NOT NULL CHECK (jsonb_typeof(overlap_lines) = 'array'),
    regular_lines JSONB NOT NULL CHECK (jsonb_typeof(regular_lines) = 'array'),
    summary_text TEXT NOT NULL,
    summary_text_en TEXT NOT NULL,
    language TEXT NOT NULL,
    keywords JSONB NOT NULL CHECK (jsonb_typeof(keywords) = 'array'),
    keywords_en JSONB NOT NULL CHECK (jsonb_typeof(keywords_en) = 'array'),
    units JSONB NOT NULL CHECK (jsonb_typeof(units) = 'array'),
    empty_reason TEXT NOT NULL,
    completeness_review JSONB NOT NULL,
    correctness_review JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (skill_name, skill_version, model_name, run_id, summary_seq_no),
    UNIQUE (skill_name, skill_version, model_name, run_id, summary_id),
    FOREIGN KEY (skill_name, skill_version, model_name, run_id, input_record_id)
        REFERENCES testbed.summary_benchmark_runs
        (skill_name, skill_version, model_name, run_id, input_record_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_testbed_summaries_input_run
    ON testbed.summaries (input_record_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS testbed.summaries;
DROP TABLE IF EXISTS testbed.summary_benchmark_runs;
