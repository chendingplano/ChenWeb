-- +goose Up
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT 'element', v.id, 1, '0.4', v.body, v.status, 'seed:wpi-emoji-picker', 'seed:wpi-emoji-picker'
FROM (VALUES
('emoji-picker', 'active', $wpi${
  "descriptor_id": "emoji-picker",
  "schema_version": "0.4",
  "code_module_id": "value-editor",
  "description": {"en": "An emoji found by English or Chinese keywords", "zh-cn": "通过中英文关键词选择表情"},
  "implement_notes": {"en": "KnowledgeStore/doc-repo/code_modules/interpreter-value-editor.md"},
  "editor": {"scope": "class", "help_tip": {"en": "The value editor this class draws.", "zh-cn": "此类绘制的值编辑器。"}, "value_editor": {"id": "read-only", "options": {}}, "default_value": "emoji-picker"},
  "label": {"scope": "instance", "help_tip": {"en": "The caption shown above the editor.", "zh-cn": "显示在编辑器上方的标题。"}, "value_editor": {"id": "localized-text", "options": {}}, "default_value": {"en": "Emoji", "zh-cn": "表情"}},
  "data_binding": {"scope": "instance", "help_tip": {"en": "The input document attribute to edit.", "zh-cn": "要编辑的输入文档属性。"}, "value_editor": {"id": "text", "options": {}}},
  "binding_mode": {"scope": "instance", "help_tip": {"en": "Choose whether the value is editable.", "zh-cn": "选择是否允许编辑该值。"}, "value_editor": {"id": "value-picker", "options": {"choices": ["read-only", "read-write"]}}, "default_value": "read-write"},
  "editor_options": {"scope": "instance", "help_tip": {"en": "Options for the editor.", "zh-cn": "编辑器选项。"}, "value_editor": {"id": "json", "options": {"rows": 3}}, "default_value": {}},
  "width": {"scope": "instance", "help_tip": {"en": "Width of the editor.", "zh-cn": "编辑器宽度。"}, "value_editor": {"id": "length", "options": {"keywords": ["auto", "elastic"], "units": ["px", "rem", "em", "%"]}}, "default_value": "elastic"},
  "attribute_definitions": {"/description": {"scope": "class", "value_editor": {"id": "localized-text", "options": {}}}, "/descriptor_id": {"scope": "class", "value_editor": {"id": "text", "options": {}}}, "/code_module_id": {"scope": "class", "value_editor": {"id": "text", "options": {}}}, "/schema_version": {"scope": "class", "value_editor": {"id": "text", "options": {}}}, "/implement_notes": {"scope": "class", "value_editor": {"id": "localized-text", "options": {}}}}
}$wpi$::jsonb)
) AS v(id,status,body)
WHERE NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind='element' AND descriptor_id=v.id);

-- +goose Down
DELETE FROM kb.wpi_descriptors WHERE kind='element' AND descriptor_id='emoji-picker' AND created_by='seed:wpi-emoji-picker';
