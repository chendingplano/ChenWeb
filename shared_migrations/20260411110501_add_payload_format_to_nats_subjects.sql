-- +goose Up
ALTER TABLE IF EXISTS shared.nats_subjects
ADD COLUMN IF NOT EXISTS payload_format TEXT DEFAULT NULL;

-- +goose Down
ALTER TABLE IF EXISTS shared.nats_subjects
DROP COLUMN IF EXISTS payload_format;
