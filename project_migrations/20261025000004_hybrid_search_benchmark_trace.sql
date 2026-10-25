-- +goose Up
-- Every query text a benchmark question searched (openspec hybrid-search-benchmark-trace): the
-- question as written (seq 1) and, for medium/high runs, each query the effort loop searched.
-- attempt is the effort attempt (1-based); NULL for the question and for the fallback search after
-- a failed effort analysis. keywords are the BM25 keywords that query used.
CREATE TABLE IF NOT EXISTS testbed.hybrid_search_benchmark_queries (
    id BIGSERIAL PRIMARY KEY,
    result_id BIGINT NOT NULL REFERENCES testbed.hybrid_search_benchmark_results(id) ON DELETE CASCADE,
    seq INTEGER NOT NULL CHECK (seq > 0),
    source TEXT NOT NULL CHECK (source IN ('question', 'effort')),
    attempt INTEGER CHECK (attempt > 0),
    query_text TEXT NOT NULL,
    keywords TEXT[] NOT NULL DEFAULT '{}',
    error_msg TEXT NOT NULL DEFAULT '',
    UNIQUE (result_id, seq)
);

-- The candidate lists one query's hybrid search built, read from the search's own CTEs:
--   lexical_partition  each partition's BM25 list; score = BM25, stage_rank = rank in the partition
--   lexical            merged lexical list; score = BM25 (ts_rank_cd on the Postgres backend),
--                      stage_rank = merged rank, partition_rank = rank in its partition
--   semantic           score = cosine similarity (1 - distance), stage_rank = semantic rank
--   fused              score = RRF, stage_rank = final order, lexical_rank/semantic_rank = input ranks
CREATE TABLE IF NOT EXISTS testbed.hybrid_search_benchmark_candidates (
    query_id BIGINT NOT NULL REFERENCES testbed.hybrid_search_benchmark_queries(id) ON DELETE CASCADE,
    stage TEXT NOT NULL CHECK (stage IN ('lexical_partition', 'lexical', 'semantic', 'fused')),
    partition_name TEXT NOT NULL,
    stage_rank INTEGER NOT NULL CHECK (stage_rank > 0),
    artifact_type TEXT NOT NULL,
    artifact_id TEXT NOT NULL,
    score DOUBLE PRECISION NOT NULL,
    partition_rank INTEGER CHECK (partition_rank > 0),
    lexical_rank INTEGER CHECK (lexical_rank > 0),
    semantic_rank INTEGER CHECK (semantic_rank > 0),
    PRIMARY KEY (query_id, stage, partition_name, stage_rank)
);

-- +goose Down
DROP TABLE IF EXISTS testbed.hybrid_search_benchmark_candidates;
DROP TABLE IF EXISTS testbed.hybrid_search_benchmark_queries;
