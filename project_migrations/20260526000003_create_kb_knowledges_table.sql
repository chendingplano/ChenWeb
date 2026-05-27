-- +goose Up
CREATE TABLE IF NOT EXISTS kb.knowledges (
    id BIGSERIAL PRIMARY KEY,
    event_id TEXT,
    input_record_id BIGINT NOT NULL,
    knowledge_id TEXT,
    language TEXT,
    knowledge_type TEXT,
    knowledge_value TEXT,
    knowledge_value_en TEXT,
    desc_text TEXT,
    desc_text_en TEXT,
    keywords JSONB,
    keywords_en JSONB,
    lines JSONB,
    category_paths JSONB,
    category_paths_en JSONB,
    model_name TEXT,
    prompt_name TEXT,
    search_document TEXT,
    search_vector TSVECTOR,
    ext_info JSONB,
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_knowledges_input_record_id ON kb.knowledges (input_record_id);
CREATE INDEX IF NOT EXISTS idx_kb_knowledges_knowledge_id ON kb.knowledges (knowledge_id);

-- +goose Down
DROP INDEX IF EXISTS idx_kb_knowledges_knowledge_id;
DROP INDEX IF EXISTS idx_kb_knowledges_input_record_id;
DROP TABLE IF EXISTS kb.knowledges;
