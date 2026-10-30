-- +goose Up
CREATE SCHEMA IF NOT EXISTS archive;
CREATE TABLE IF NOT EXISTS archive.frontend_pages (
    id BIGSERIAL PRIMARY KEY,
    descriptor_id TEXT NOT NULL,
    version INTEGER NOT NULL,
    schema_version TEXT NOT NULL,
    body JSONB NOT NULL,
    status TEXT NOT NULL,
    created_by TEXT NOT NULL DEFAULT '',
    updated_by TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    recovery BOOLEAN NOT NULL DEFAULT FALSE,
    owner TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS frontend_pages_recovery_owner ON archive.frontend_pages (descriptor_id, owner) WHERE recovery;

-- +goose Down
DROP TABLE IF EXISTS archive.frontend_pages;
