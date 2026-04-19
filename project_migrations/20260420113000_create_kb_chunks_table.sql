-- +goose Up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.chunks (
    id BIGSERIAL PRIMARY KEY,
    source_record_id BIGINT NOT NULL REFERENCES kb.inputs(id) ON DELETE CASCADE,
    chunking_method VARCHAR(64) NOT NULL DEFAULT 'fix-size',
    chunking_size INTEGER NOT NULL DEFAULT 300,
    overlap_percent INTEGER NOT NULL DEFAULT 20,
    notes TEXT NOT NULL DEFAULT '',
    create_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    update_time TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_kb_chunks_source_record_id
    ON kb.chunks(source_record_id);

CREATE INDEX IF NOT EXISTS idx_kb_chunks_create_time
    ON kb.chunks(create_time);

-- +goose Down
DROP TABLE IF EXISTS kb.chunks;
