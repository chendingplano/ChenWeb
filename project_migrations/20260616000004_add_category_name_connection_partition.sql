-- +goose Up
CREATE TABLE IF NOT EXISTS kb.artifact_connections_category_name
    PARTITION OF kb.artifact_connections
    FOR VALUES IN ('category_name');

-- +goose Down
DROP TABLE IF EXISTS kb.artifact_connections_category_name;
