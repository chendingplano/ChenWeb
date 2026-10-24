-- +goose Up
CREATE SCHEMA IF NOT EXISTS testbed;

-- One row per benchmark run, with the search settings that produced its ranks.
CREATE TABLE IF NOT EXISTS testbed.hybrid_search_benchmark_runs (
    id BIGSERIAL PRIMARY KEY,
    purpose TEXT NOT NULL CHECK (btrim(purpose) <> ''),
    description TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'done', 'failed')),
    error_msg TEXT NOT NULL DEFAULT '',
    question_count INTEGER NOT NULL DEFAULT 0 CHECK (question_count >= 0),
    done_count INTEGER NOT NULL DEFAULT 0 CHECK (done_count >= 0),
    lexical_backend TEXT NOT NULL DEFAULT '',
    query_keywords BOOLEAN NOT NULL DEFAULT FALSE,
    embedding_model TEXT NOT NULL DEFAULT '',
    lexical_weight DOUBLE PRECISION NOT NULL DEFAULT 1,
    semantic_weight DOUBLE PRECISION NOT NULL DEFAULT 1,
    candidate_limit INTEGER NOT NULL DEFAULT 200,
    created_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL DEFAULT (NOW() + INTERVAL '10 minutes')
);
CREATE INDEX IF NOT EXISTS idx_hybrid_search_benchmark_runs_created
    ON testbed.hybrid_search_benchmark_runs (created_at DESC);

-- One row per question per run. The question is copied so a run stays readable after the
-- generated question is deleted. Ranks are 1-based within the first candidate_limit results;
-- NULL means the target was not found.
CREATE TABLE IF NOT EXISTS testbed.hybrid_search_benchmark_results (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES testbed.hybrid_search_benchmark_runs(id) ON DELETE CASCADE,
    question_id BIGINT REFERENCES testbed.generated_questions(id) ON DELETE SET NULL,
    input_record_id BIGINT NOT NULL,
    hop_type TEXT NOT NULL,
    lang TEXT NOT NULL,
    question TEXT NOT NULL,
    target_artifact_id TEXT NOT NULL,
    keywords TEXT[] NOT NULL DEFAULT '{}',
    lexical_rank INTEGER CHECK (lexical_rank > 0),
    semantic_rank INTEGER CHECK (semantic_rank > 0),
    hybrid_rank INTEGER CHECK (hybrid_rank > 0),
    hybrid_top JSONB NOT NULL DEFAULT '[]'::jsonb CHECK (jsonb_typeof(hybrid_top) = 'array'),
    error_msg TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (run_id, question_id)
);
CREATE INDEX IF NOT EXISTS idx_hybrid_search_benchmark_results_run
    ON testbed.hybrid_search_benchmark_results (run_id, id);

-- +goose Down
DROP TABLE IF EXISTS testbed.hybrid_search_benchmark_results;
DROP TABLE IF EXISTS testbed.hybrid_search_benchmark_runs;
