-- +goose Up
CREATE TABLE IF NOT EXISTS kb.artifact_connections_object_id
    PARTITION OF kb.artifact_connections
    FOR VALUES IN ('object_id');

-- +goose Down
DROP TABLE IF EXISTS kb.artifact_connections_object_id;
