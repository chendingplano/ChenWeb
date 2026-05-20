-- +goose Up
ALTER TABLE kb.products
    ADD COLUMN IF NOT EXISTS confidence_reason_en TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE kb.products
    DROP COLUMN IF EXISTS confidence_reason_en;
