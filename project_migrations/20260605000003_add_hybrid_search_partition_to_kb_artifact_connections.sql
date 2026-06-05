-- +goose Up
CREATE TABLE IF NOT EXISTS kb.artifact_connections_hybrid_search
    PARTITION OF kb.artifact_connections FOR VALUES IN ('hybrid_search');

-- +goose Down
DROP TABLE IF EXISTS kb.artifact_connections_hybrid_search;
