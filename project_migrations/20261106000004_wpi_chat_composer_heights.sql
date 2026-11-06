-- +goose Up
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT kind, descriptor_id,
  (SELECT COALESCE(MAX(version), 0) + 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'chat-composer'),
  schema_version, body || $attrs${
    "message_box_height": {"scope":"instance", "help_tip":{"en":"Message box height; leave unset to use rows.","zh-cn":"消息框高度；留空时使用行数。"}, "value_editor":{"id":"length","options":{"units":["px","rem","em","%"]}}},
    "toolbar_height": {"scope":"instance", "help_tip":{"en":"Toolbar height; leave unset for automatic sizing.","zh-cn":"工具栏高度；留空时自动调整。"}, "value_editor":{"id":"length","options":{"units":["px","rem","em","%"]}}}
  }$attrs$::jsonb, 'active', 'seed:wpi-chat-composer-heights', 'seed:wpi-chat-composer-heights'
FROM kb.wpi_descriptors
WHERE kind = 'element' AND descriptor_id = 'chat-composer' AND status = 'active'
  AND version = (SELECT MAX(version) FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'chat-composer' AND status = 'active')
  AND NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'chat-composer' AND created_by = 'seed:wpi-chat-composer-heights');

-- +goose Down
DELETE FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'chat-composer' AND created_by = 'seed:wpi-chat-composer-heights';
