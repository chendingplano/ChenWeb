-- +goose Up
-- Keyword model and prompt a run used (empty when query keywords were off).
ALTER TABLE testbed.hybrid_search_benchmark_runs
    ADD COLUMN IF NOT EXISTS keywords_model TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS keywords_prompt TEXT NOT NULL DEFAULT '';

-- Per-question timings in milliseconds: query embedding, keyword extraction (always a fresh LLM
-- call, never the cache) and each mode's search query. NULL when the step did not run.
ALTER TABLE testbed.hybrid_search_benchmark_results
    ADD COLUMN IF NOT EXISTS embedding_ms INTEGER,
    ADD COLUMN IF NOT EXISTS keywords_ms INTEGER,
    ADD COLUMN IF NOT EXISTS lexical_ms INTEGER,
    ADD COLUMN IF NOT EXISTS semantic_ms INTEGER,
    ADD COLUMN IF NOT EXISTS hybrid_ms INTEGER;

-- +goose Down
ALTER TABLE testbed.hybrid_search_benchmark_results
    DROP COLUMN IF EXISTS hybrid_ms,
    DROP COLUMN IF EXISTS semantic_ms,
    DROP COLUMN IF EXISTS lexical_ms,
    DROP COLUMN IF EXISTS keywords_ms,
    DROP COLUMN IF EXISTS embedding_ms;
ALTER TABLE testbed.hybrid_search_benchmark_runs
    DROP COLUMN IF EXISTS keywords_prompt,
    DROP COLUMN IF EXISTS keywords_model;
