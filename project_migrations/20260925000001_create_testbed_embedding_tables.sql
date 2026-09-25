-- +goose Up
CREATE SCHEMA IF NOT EXISTS testbed;

CREATE TABLE IF NOT EXISTS testbed.embedding_768 (
    id BIGSERIAL PRIMARY KEY,
    model_key TEXT NOT NULL,
    model_name TEXT NOT NULL,
    content TEXT NOT NULL,
    embedding vector(768) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS embedding_768_model_id_idx
    ON testbed.embedding_768 (model_key, id DESC);

CREATE TABLE IF NOT EXISTS testbed.embedding_1024 (
    id BIGSERIAL PRIMARY KEY,
    model_key TEXT NOT NULL,
    model_name TEXT NOT NULL,
    content TEXT NOT NULL,
    embedding vector(1024) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS embedding_1024_model_id_idx
    ON testbed.embedding_1024 (model_key, id DESC);

CREATE TABLE IF NOT EXISTS testbed.embedding_1536 (
    id BIGSERIAL PRIMARY KEY,
    model_key TEXT NOT NULL,
    model_name TEXT NOT NULL,
    content TEXT NOT NULL,
    embedding vector(1536) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS embedding_1536_model_id_idx
    ON testbed.embedding_1536 (model_key, id DESC);

-- +goose Down
DROP TABLE IF EXISTS testbed.embedding_1536;
DROP TABLE IF EXISTS testbed.embedding_1024;
DROP TABLE IF EXISTS testbed.embedding_768;
