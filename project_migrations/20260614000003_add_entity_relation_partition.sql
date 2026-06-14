-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kb.artifact_connections_entity_relation
    PARTITION OF kb.artifact_connections
    FOR VALUES IN ('entity_relation');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS kb.artifact_connections_entity_relation;
-- +goose StatementEnd
