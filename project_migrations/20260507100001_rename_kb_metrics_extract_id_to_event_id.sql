-- +goose Up
ALTER TABLE kb.metrics
    RENAME COLUMN extract_id TO event_id;
ALTER TABLE kb.metrics
    ALTER COLUMN event_id DROP NOT NULL;

-- +goose Down
ALTER TABLE kb.metrics
    ALTER COLUMN event_id SET NOT NULL;
ALTER TABLE kb.metrics
    RENAME COLUMN event_id TO extract_id;
