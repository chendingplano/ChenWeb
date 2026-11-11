-- +goose Up
-- Benchmark questions can target any search artifact type (openspec hybrid-search-benchmark-purposes).
-- Existing results were all metric questions.
ALTER TABLE testbed.hybrid_search_benchmark_results
    ADD COLUMN IF NOT EXISTS target_artifact_type TEXT NOT NULL DEFAULT 'metric';

-- +goose Down
-- Rollback refuses while results of another type exist, because the old schema assumes metrics.
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM testbed.hybrid_search_benchmark_results WHERE target_artifact_type <> 'metric') THEN
        RAISE EXCEPTION 'cannot roll back: benchmark results with non-metric targets exist';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE testbed.hybrid_search_benchmark_results DROP COLUMN IF EXISTS target_artifact_type;
