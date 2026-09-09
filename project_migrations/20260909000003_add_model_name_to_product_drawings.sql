-- +goose Up
ALTER TABLE IF EXISTS kb.product_drawings
    ADD COLUMN IF NOT EXISTS model_name TEXT NOT NULL DEFAULT 'openai-image-2.5';

-- +goose Down
ALTER TABLE IF EXISTS kb.product_drawings
    DROP COLUMN IF EXISTS model_name;
