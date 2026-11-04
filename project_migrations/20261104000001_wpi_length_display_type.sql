-- +goose Up
-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors
    (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT kind, descriptor_id, version + 1, schema_version,
       jsonb_set(body, '{editor_options,default_value,displayType}', 'false'::jsonb, true),
       'active', 'migration:wpi-length-display-type', 'migration:wpi-length-display-type'
FROM (
    SELECT * FROM kb.wpi_descriptors
    WHERE kind = 'element' AND descriptor_id = 'length-editor'
    ORDER BY version DESC LIMIT 1
) AS latest
WHERE status = 'active'
  AND NOT (body->'editor_options'->'default_value' ? 'displayType');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM kb.wpi_descriptors
WHERE kind = 'element' AND descriptor_id = 'length-editor'
  AND created_by = 'migration:wpi-length-display-type';
-- +goose StatementEnd
