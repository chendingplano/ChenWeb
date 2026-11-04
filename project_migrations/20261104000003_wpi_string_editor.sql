-- +goose Up
-- The class has its own identity so saved text-editor instances remain unchanged.
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT 'element', 'string-editor', 1, '0.4',
 jsonb_set(jsonb_set(jsonb_set(jsonb_set(body, '{descriptor_id}', '"string-editor"'::jsonb),
  '{description}', '{"en":"String value editor","zh-cn":"字符串值编辑器"}'::jsonb),
  '{editor,default_value}', '"string-editor"'::jsonb),
  '{editor_options,default_value}', '{"mode":"single","languages":["en","zh-cn"],"emptyAsNull":true,"choices":[],"commitOnBlur":false}'::jsonb),
 'draft', 'seed:wpi-string-editor', 'seed:wpi-string-editor'
FROM kb.wpi_descriptors
WHERE kind = 'element' AND descriptor_id = 'text-editor' AND status = 'active'
  AND NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'string-editor')
ORDER BY version DESC LIMIT 1;

-- Promote the checked implementation with a new immutable descriptor version.
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT kind, descriptor_id, 2, schema_version, body, 'active', 'seed:wpi-string-editor', 'seed:wpi-string-editor'
FROM kb.wpi_descriptors d
WHERE kind = 'element' AND descriptor_id = 'string-editor' AND version = 1
  AND NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors x WHERE x.kind=d.kind AND x.descriptor_id=d.descriptor_id AND x.version >= 2);

-- +goose Down
DELETE FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'string-editor'
 AND version IN (1, 2) AND created_by = 'seed:wpi-string-editor';
