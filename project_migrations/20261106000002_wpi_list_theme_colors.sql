-- +goose Up
-- Enable the shared picker's light/dark mode in a new list class version.
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT kind, descriptor_id,
  (SELECT COALESCE(MAX(version), 0) + 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'list'),
  schema_version,
  jsonb_set(jsonb_set(jsonb_set(jsonb_set(body,
    '{background_color,value_editor,options,theme}', 'true'::jsonb),
    '{font_color,value_editor,options,theme}', 'true'::jsonb),
    '{odd_row_background_color,value_editor,options,theme}', 'true'::jsonb),
    '{even_row_background_color,value_editor,options,theme}', 'true'::jsonb),
  'active', 'seed:wpi-list-theme-colors', 'seed:wpi-list-theme-colors'
FROM kb.wpi_descriptors
WHERE kind = 'element' AND descriptor_id = 'list' AND status = 'active'
  AND version = (SELECT MAX(version) FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'list' AND status = 'active')
  AND body ?& array['background_color', 'font_color', 'odd_row_background_color', 'even_row_background_color']
  AND NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'list' AND created_by = 'seed:wpi-list-theme-colors');

-- +goose Down
DELETE FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'list' AND created_by = 'seed:wpi-list-theme-colors';
