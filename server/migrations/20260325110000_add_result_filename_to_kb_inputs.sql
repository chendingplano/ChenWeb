-- +goose Up
ALTER TABLE kb.inputs
ADD COLUMN IF NOT EXISTS result_filename TEXT;

-- +goose Down
ALTER TABLE kb.inputs
DROP COLUMN IF EXISTS result_filename;
