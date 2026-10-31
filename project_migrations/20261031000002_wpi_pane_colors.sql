-- +goose Up
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT d.kind, d.descriptor_id, d.version + 1, d.schema_version,
       jsonb_set(d.body, '{root}', (d.body->'root') || '{"background_color":null,"foreground_color":null}'::jsonb),
       'active', 'migration:pane-colors', 'migration:pane-colors'
FROM kb.wpi_descriptors d
WHERE d.kind = 'page-layouter' AND d.status = 'active'
  AND d.version = (SELECT max(x.version) FROM kb.wpi_descriptors x WHERE x.kind=d.kind AND x.descriptor_id=d.descriptor_id)
  AND NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors x WHERE x.kind=d.kind AND x.descriptor_id=d.descriptor_id AND x.created_by='migration:pane-colors');

-- +goose Down
DELETE FROM kb.wpi_descriptors WHERE created_by = 'migration:pane-colors';
