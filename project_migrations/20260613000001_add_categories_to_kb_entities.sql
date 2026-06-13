-- +goose Up
ALTER TABLE kb.entities ADD COLUMN IF NOT EXISTS categories JSONB;

-- +goose Down
ALTER TABLE kb.entities DROP COLUMN IF EXISTS categories;
