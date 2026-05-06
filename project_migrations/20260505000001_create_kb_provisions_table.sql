-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.provisions (
    id BIGSERIAL PRIMARY KEY,
    input_record_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    extract_id TEXT NOT NULL,
    input_filename TEXT NOT NULL,
    prov_id INTEGER NOT NULL,
    prov_name TEXT,
    provision_type TEXT,
    source_text TEXT,
    source_line_spans JSONB NOT NULL DEFAULT '[]'::jsonb,
    provision_original TEXT,
    provision_en TEXT,
    provision_subject TEXT,
    prov_desc TEXT,
    prov_context TEXT,
    provision_keywords JSONB NOT NULL DEFAULT '[]'::jsonb,
    category_paths JSONB NOT NULL DEFAULT '[]'::jsonb,
    location_type TEXT,
    confidence DOUBLE PRECISION,
    is_explicit BOOLEAN,
    need_verify BOOLEAN,
    num_blocks INTEGER,
    num_provisions INTEGER,
    time_per_provision DOUBLE PRECISION,
    model_name TEXT,
    prompt_name TEXT,
    status TEXT NOT NULL DEFAULT 'active',
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    modify_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    public_info JSONB NOT NULL DEFAULT '{}'::jsonb,
    private_info JSONB NOT NULL DEFAULT '{}'::jsonb,
    notes TEXT NOT NULL DEFAULT '',
    error_msg TEXT NOT NULL DEFAULT '',
    CONSTRAINT uq_kb_provisions_input_prov_id UNIQUE (input_record_id, prov_id)
);

CREATE INDEX IF NOT EXISTS idx_kb_provisions_input_record_id
    ON kb.provisions(input_record_id);

CREATE INDEX IF NOT EXISTS idx_kb_provisions_extract_id
    ON kb.provisions(extract_id);

CREATE INDEX IF NOT EXISTS idx_kb_provisions_status
    ON kb.provisions(status);

CREATE INDEX IF NOT EXISTS idx_kb_provisions_keywords_gin
    ON kb.provisions USING GIN (provision_keywords);

CREATE INDEX IF NOT EXISTS idx_kb_provisions_categories_gin
    ON kb.provisions USING GIN (category_paths);

-- +goose Down
DROP INDEX IF EXISTS kb.idx_kb_provisions_categories_gin;
DROP INDEX IF EXISTS kb.idx_kb_provisions_keywords_gin;
DROP INDEX IF EXISTS kb.idx_kb_provisions_status;
DROP INDEX IF EXISTS kb.idx_kb_provisions_extract_id;
DROP INDEX IF EXISTS kb.idx_kb_provisions_input_record_id;
DROP TABLE IF EXISTS kb.provisions;
