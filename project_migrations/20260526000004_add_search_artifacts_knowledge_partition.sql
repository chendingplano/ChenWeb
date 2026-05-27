-- +goose Up
CREATE TABLE IF NOT EXISTS kb.search_artifacts_knowledge PARTITION OF kb.search_artifacts
    FOR VALUES IN ('knowledge');

CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_knowledge_search_vector
    ON kb.search_artifacts_knowledge USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_knowledge_record
    ON kb.search_artifacts_knowledge (input_record_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_search_artifacts_knowledge_record;
DROP INDEX IF EXISTS idx_kb_search_artifacts_knowledge_search_vector;
DROP TABLE IF EXISTS kb.search_artifacts_knowledge;
