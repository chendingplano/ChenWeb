-- +goose Up
CREATE TABLE IF NOT EXISTS kb.search_artifacts_semantic_projection PARTITION OF kb.search_artifacts
    FOR VALUES IN ('semantic_projection');

CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_semantic_projection_search_vector
    ON kb.search_artifacts_semantic_projection USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_semantic_projection_record
    ON kb.search_artifacts_semantic_projection (input_record_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_search_artifacts_semantic_projection_record;
DROP INDEX IF EXISTS idx_kb_search_artifacts_semantic_projection_search_vector;
DROP TABLE IF EXISTS kb.search_artifacts_semantic_projection;
