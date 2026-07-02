-- +goose Up
CREATE TABLE IF NOT EXISTS kb.artifact_connections_line_overlapped_artifact
    PARTITION OF kb.artifact_connections
    FOR VALUES IN ('line-overlapped-artifact');

-- +goose Down
DROP TABLE IF EXISTS kb.artifact_connections_line_overlapped_artifact;
