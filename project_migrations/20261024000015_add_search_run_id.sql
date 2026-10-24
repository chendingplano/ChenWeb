-- +goose Up
-- Each search (search API, agent search, benchmark question) takes one value as the run ID of
-- its LLM calls (query embedding, query keywords) in public.llm_usage_event.
CREATE SEQUENCE IF NOT EXISTS kb.search_run_id_seq;

ALTER TABLE testbed.hybrid_search_benchmark_results
    ADD COLUMN IF NOT EXISTS search_run_id BIGINT;

-- +goose Down
ALTER TABLE testbed.hybrid_search_benchmark_results
    DROP COLUMN IF EXISTS search_run_id;
DROP SEQUENCE IF EXISTS kb.search_run_id_seq;
