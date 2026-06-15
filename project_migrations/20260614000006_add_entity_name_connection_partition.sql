-- +goose Up
-- Change 02 adds relation_method='entity_name' edges:
-- (entity_name:name_key) --has-instance--> (entity:entity_id).
-- kb.artifact_connections is partitioned by relation_method, so inserts need a
-- matching partition.
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS kb.artifact_connections_entity_name
    PARTITION OF kb.artifact_connections
    FOR VALUES IN ('entity_name');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS kb.artifact_connections_entity_name;
-- +goose StatementEnd
