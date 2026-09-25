-- Switch the stored embedding dimension from 1536 (text-embedding-3-small) to
-- 1024 (bge-m3). Vectors from different models are not comparable, so existing
-- embeddings cannot be converted: every column is reset to NULL and must be
-- re-embedded (POST /kb/search/backfill-embeddings with reembed_all,
-- class-contract-search-backfill --reembed-all; kb.product_profile_nodes refills
-- lazily during grounding). Until then those rows fall back to lexical search.
--
-- ALTER TYPE on the partitioned kb.search_artifacts parent propagates to every
-- partition, and Postgres rebuilds the per-partition HNSW indexes (and the one on
-- kb.ontology_class_contract_search) as part of the rewrite.

-- +goose Up
ALTER TABLE kb.search_artifacts
    ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);
ALTER TABLE kb.ontology_class_contract_search
    ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);
ALTER TABLE kb.product_profile_nodes
    ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);
ALTER TABLE kb.object_nodes
    ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);
ALTER TABLE kb.cdm_projections
    ALTER COLUMN embedding TYPE vector(1024) USING NULL::vector(1024);

-- +goose Down
ALTER TABLE kb.cdm_projections
    ALTER COLUMN embedding TYPE vector(1536) USING NULL::vector(1536);
ALTER TABLE kb.object_nodes
    ALTER COLUMN embedding TYPE vector(1536) USING NULL::vector(1536);
ALTER TABLE kb.product_profile_nodes
    ALTER COLUMN embedding TYPE vector(1536) USING NULL::vector(1536);
ALTER TABLE kb.ontology_class_contract_search
    ALTER COLUMN embedding TYPE vector(1536) USING NULL::vector(1536);
ALTER TABLE kb.search_artifacts
    ALTER COLUMN embedding TYPE vector(1536) USING NULL::vector(1536);
