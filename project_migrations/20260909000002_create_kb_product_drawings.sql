-- +goose up
CREATE SCHEMA IF NOT EXISTS kb;

CREATE TABLE IF NOT EXISTS kb.product_drawings (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    prompt TEXT NOT NULL,
    keywords TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    filename TEXT NOT NULL UNIQUE,
    stored_path TEXT NOT NULL,
    model TEXT NOT NULL DEFAULT 'openai-image-2.5',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS product_drawings_created_at_idx
    ON kb.product_drawings (created_at DESC, id DESC);

-- +goose down
DROP TABLE IF EXISTS kb.product_drawings;
