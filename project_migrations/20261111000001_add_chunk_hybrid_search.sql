-- +goose Up
CREATE TABLE IF NOT EXISTS kb.search_artifacts_chunk PARTITION OF kb.search_artifacts FOR VALUES IN ('chunk');
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_chunk_search_vector ON kb.search_artifacts_chunk USING GIN (search_vector);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_chunk_record ON kb.search_artifacts_chunk (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_search_artifacts_chunk_embedding_hnsw ON kb.search_artifacts_chunk USING hnsw (embedding vector_cosine_ops);

CREATE TABLE IF NOT EXISTS kb.chunk_search_state (
    input_record_id BIGINT PRIMARY KEY REFERENCES kb.inputs(id) ON DELETE CASCADE,
    generation BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS kb.chunk_search_windows (
    artifact_type TEXT NOT NULL DEFAULT 'chunk' CHECK (artifact_type = 'chunk'),
    artifact_id TEXT NOT NULL,
    window_index INTEGER NOT NULL CHECK (window_index > 0),
    embedding_text TEXT NOT NULL,
    embedding vector(1024),
    PRIMARY KEY (artifact_id, window_index),
    FOREIGN KEY (artifact_type, artifact_id) REFERENCES kb.search_artifacts(artifact_type, artifact_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_kb_chunk_search_windows_embedding_hnsw ON kb.chunk_search_windows USING hnsw (embedding vector_cosine_ops);

-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_search') THEN
        CREATE INDEX IF NOT EXISTS kb_search_artifacts_chunk_bm25 ON kb.search_artifacts_chunk
        USING bm25 (artifact_id, search_document, primary_label, secondary_label, snippet_basis, source_title, source_filename)
        WITH (key_field = 'artifact_id', text_fields = '{
            "search_document":{"tokenizer":{"type":"jieba"}},
            "primary_label":{"tokenizer":{"type":"jieba"}},
            "secondary_label":{"tokenizer":{"type":"jieba"}},
            "snippet_basis":{"tokenizer":{"type":"jieba"}},
            "source_title":{"tokenizer":{"type":"jieba"}},
            "source_filename":{"tokenizer":{"type":"jieba"}}
        }');
    END IF;
END $$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION kb.invalidate_changed_input_chunk_search() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO kb.chunk_search_state (input_record_id, generation) VALUES (NEW.id, 1)
    ON CONFLICT (input_record_id) DO UPDATE SET generation = kb.chunk_search_state.generation + 1;
    DELETE FROM kb.search_artifacts WHERE artifact_type = 'chunk' AND input_record_id = NEW.id;
    RETURN NEW;
END $$;
-- +goose StatementEnd

DROP TRIGGER IF EXISTS invalidate_changed_input_chunk_search ON kb.inputs;
CREATE TRIGGER invalidate_changed_input_chunk_search AFTER UPDATE OF result_filename, staging_filename, parser_name ON kb.inputs
FOR EACH ROW WHEN (OLD.result_filename IS DISTINCT FROM NEW.result_filename OR OLD.staging_filename IS DISTINCT FROM NEW.staging_filename OR OLD.parser_name IS DISTINCT FROM NEW.parser_name)
EXECUTE FUNCTION kb.invalidate_changed_input_chunk_search();

-- +goose Down
DROP TRIGGER IF EXISTS invalidate_changed_input_chunk_search ON kb.inputs;
DROP FUNCTION IF EXISTS kb.invalidate_changed_input_chunk_search();
DROP TABLE IF EXISTS kb.chunk_search_windows;
DROP TABLE IF EXISTS kb.chunk_search_state;
DROP TABLE IF EXISTS kb.search_artifacts_chunk;
