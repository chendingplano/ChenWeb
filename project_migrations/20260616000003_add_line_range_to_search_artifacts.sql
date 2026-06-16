-- +goose Up
-- Add line_range to the unified search-artifact registry so the line-overlap
-- artifact<->artifact self-join (step 3) can run as a single query over kb.search_artifacts
-- instead of per-family base-table joins. See
-- KnowledgeStore/Capsules/coding-capsules/llm-wiki/artifact-connections.md §2.2.
--
-- kb.search_artifacts is partitioned LIST (artifact_type); the generated column and the
-- GiST index propagate to every partition. NOTE: adding a STORED generated column rewrites
-- each partition and rebuilds its indexes (including the hnsw embedding index), so this
-- migration is slower than the base-table rollouts. Reuses kb.line_spans_to_int8multirange
-- (created by 20260616000001).

ALTER TABLE kb.search_artifacts
    ADD COLUMN IF NOT EXISTS line_range int8multirange
    GENERATED ALWAYS AS (kb.line_spans_to_int8multirange(source_line_spans)) STORED;

CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_line_range
    ON kb.search_artifacts USING gist (line_range);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_search_artifacts_line_range;
ALTER TABLE kb.search_artifacts DROP COLUMN IF EXISTS line_range;
