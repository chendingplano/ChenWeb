-- +goose Up
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT kind, descriptor_id,
  (SELECT COALESCE(MAX(version), 0) + 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'button'),
  schema_version, body || $attrs${
    "background_color": {"scope":"instance", "help_tip":{"en":"Button background; leave unset to use the theme.","zh-cn":"按钮背景；留空时使用主题颜色。"}, "value_editor":{"id":"color","options":{"theme":true}}}
  }$attrs$::jsonb, 'active', 'seed:wpi-button-background-color', 'seed:wpi-button-background-color'
FROM kb.wpi_descriptors
WHERE kind = 'element' AND descriptor_id = 'button' AND status = 'active'
  AND version = (SELECT MAX(version) FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'button' AND status = 'active')
  AND NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'button' AND created_by = 'seed:wpi-button-background-color');

-- +goose Down
DELETE FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'button' AND created_by = 'seed:wpi-button-background-color';
