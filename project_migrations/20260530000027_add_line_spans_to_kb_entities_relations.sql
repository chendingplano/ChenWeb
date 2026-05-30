-- +goose Up
ALTER TABLE kb.entities
    ADD COLUMN IF NOT EXISTS line_spans JSONB;

ALTER TABLE kb.relations
    ADD COLUMN IF NOT EXISTS line_spans JSONB;

-- +goose Down
ALTER TABLE kb.relations
    DROP COLUMN IF EXISTS line_spans;

ALTER TABLE kb.entities
    DROP COLUMN IF EXISTS line_spans;
