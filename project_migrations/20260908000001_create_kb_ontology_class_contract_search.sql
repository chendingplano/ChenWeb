-- +goose Up
-- Hybrid-search index over governed class contracts (openspec change
-- analysis-node-related-metrics). One row per class term: a lexical tsvector
-- plus a pgvector embedding, fused with Reciprocal Rank Fusion by
-- classcontractsearch.MatchSimilar to power the Metric Ontology Explorer's
-- "Metrics of Similar Classes" node.
--
-- Requires the "vector" extension (installed by 20260603000001). This table is
-- deliberately NOT partitioned and NOT part of kb.search_artifacts: a class
-- term is global and has no input record, and kb.search_artifacts' writer and
-- per-record reindex/delete helpers are all record-scoped. The class corpus is
-- small (hundreds today), so a plain table with a GIN + HNSW index is enough.

CREATE TABLE IF NOT EXISTS kb.ontology_class_contract_search (
    class_term_id                 TEXT PRIMARY KEY
                                      REFERENCES kb.ontology_term_headers (term_id),
    current_contract_revision_id  BIGINT,
    definition_state              TEXT NOT NULL DEFAULT 'identity_only',
    module_id                     TEXT,
    search_document               TEXT NOT NULL DEFAULT '',
    search_vector                 tsvector,
    embedding_text                TEXT,
    embedding                     vector(1536),
    instance_count                INT NOT NULL DEFAULT 0,
    updated_at                    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_ontology_class_contract_search_vector
    ON kb.ontology_class_contract_search USING gin (search_vector);

CREATE INDEX IF NOT EXISTS idx_kb_ontology_class_contract_search_embedding_hnsw
    ON kb.ontology_class_contract_search USING hnsw (embedding vector_cosine_ops);

-- +goose Down
DROP TABLE IF EXISTS kb.ontology_class_contract_search;
-- NOTE: the "vector" extension is shared infrastructure; not dropped here.
