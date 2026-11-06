-- +goose Up
-- Add a new class version; pages keep their existing embedded class copies.
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT kind, descriptor_id,
  (SELECT COALESCE(MAX(version), 0) + 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'list'),
  schema_version, body || $attrs${
    "background_color": {"scope":"instance", "help_tip":{"en":"Background of the list.","zh-cn":"列表的背景颜色。"}, "default_value":"transparent", "value_editor":{"id":"color","options":{}}},
    "font_color": {"scope":"instance", "help_tip":{"en":"Text color of the list.","zh-cn":"列表的文字颜色。"}, "default_value":"inherit", "value_editor":{"id":"color","options":{}}},
    "odd_row_background_color": {"scope":"instance", "help_tip":{"en":"Background of odd-numbered rows, starting with the first record.","zh-cn":"单数行的背景颜色，从第一条记录开始。"}, "default_value":"transparent", "value_editor":{"id":"color","options":{}}},
    "even_row_background_color": {"scope":"instance", "help_tip":{"en":"Background of even-numbered rows.","zh-cn":"偶数行的背景颜色。"}, "default_value":"transparent", "value_editor":{"id":"color","options":{}}}
  }$attrs$::jsonb, 'active', 'seed:wpi-list-colors', 'seed:wpi-list-colors'
FROM kb.wpi_descriptors
WHERE kind = 'element' AND descriptor_id = 'list' AND status = 'active'
  AND version = (SELECT MAX(version) FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'list' AND status = 'active')
  AND NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'list' AND created_by = 'seed:wpi-list-colors');

-- +goose Down
DELETE FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'list' AND created_by = 'seed:wpi-list-colors';
