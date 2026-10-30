-- The chat-composer and chat-panel element code modules and their classes, from
-- ChenWeb's Chat page (KnowledgeStore doc-repo/code_modules/interpreter-chat-composer.md
-- and interpreter-chat-panel.md, revision R2). The two share the chat of their page
-- instance; only the harness-backed assistants are offered.
-- +goose Up
-- +goose StatementBegin
INSERT INTO kb.code_modules (code_module_id, module_name, description, module_type, module_doc, notes, creator) VALUES
    ('chat-composer', 'Chat Composer',
     'Message box and tool bar for chatting with a harness-backed AI assistant; replies appear in the chat-panel of the same page.',
     'element', 'interpreter-chat-composer.md',
     'Shares the chat of its page instance with chat-panel. At most one per page.',
     'migration:wpi-chat-elements'),
    ('chat-panel', 'Chat Panel',
     'Chat and Dialog tabs showing the conversation sent by the chat-composer of the same page.',
     'element', 'interpreter-chat-panel.md',
     'Shares the chat of its page instance with chat-composer.',
     'migration:wpi-chat-elements')
ON CONFLICT (code_module_id) DO NOTHING;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT 'element', 'chat-composer', 1, '0.4', $wpi${
  "descriptor_id": "chat-composer",
  "schema_version": "0.4",
  "code_module_id": "chat-composer",
  "description": {"en": "Message box and controls for chatting with an AI assistant", "zh-cn": "与 AI 助手聊天的消息框和控件"},
  "implement_notes": {"en": "KnowledgeStore/doc-repo/code_modules/interpreter-chat-composer.md"},
  "placeholder": {
    "help_tip": {"en": "Text shown in the empty message box.", "zh-cn": "消息框为空时显示的文字。"}
  },
  "rows": {
    "help_tip": {"en": "Visible lines of the message box, 1 to 20.", "zh-cn": "消息框可见行数，1 到 20。"},
    "default_value": 3
  },
  "slash_commands": {
    "help_tip": {"en": "Suggest slash commands when a message starts with /.", "zh-cn": "消息以 / 开头时提示斜杠命令。"},
    "default_value": true
  },
  "toolbar": {
    "help_tip": {"en": "Controls shown under the message box, in order: agent, model, prompt, text_editor, new_session.", "zh-cn": "消息框下方显示的控件及其顺序：agent、model、prompt、text_editor、new_session。"},
    "default_value": ["agent", "model", "prompt", "text_editor", "new_session"]
  },
  "agents": {
    "help_tip": {"en": "Assistants offered, in order, from Claude Code, Codex, DeepSeek Harness and pi; leave out for all four.", "zh-cn": "提供的助手及其顺序，可选 Claude Code、Codex、DeepSeek Harness 和 pi；留空则提供全部四个。"}
  },
  "default_agent": {
    "help_tip": {"en": "Assistant selected when the page opens; leave out for the first.", "zh-cn": "页面打开时选中的助手；留空则选第一个。"}
  },
  "default_model": {
    "help_tip": {"en": "Model selected when the assistant offers it.", "zh-cn": "助手提供该模型时选中它。"}
  },
  "width": {
    "help_tip": {"en": "Width of the whole composer.", "zh-cn": "整个输入区的宽度。"},
    "default_value": "elastic"
  },
  "min_width": {
    "help_tip": {"en": "Narrowest width of the composer.", "zh-cn": "输入区的最小宽度。"},
    "default_value": "320px"
  }
}$wpi$::jsonb, 'active', 'seed:wpi-chat-elements', 'seed:wpi-chat-elements'
WHERE NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'chat-composer');
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT 'element', 'chat-panel', 1, '0.4', $wpi${
  "descriptor_id": "chat-panel",
  "schema_version": "0.4",
  "code_module_id": "chat-panel",
  "description": {"en": "Conversation and history of the page's chat", "zh-cn": "页面聊天的对话和历史"},
  "implement_notes": {"en": "KnowledgeStore/doc-repo/code_modules/interpreter-chat-panel.md"},
  "tabs": {
    "help_tip": {"en": "Tabs shown, in order: chat, dialog.", "zh-cn": "显示的标签页及其顺序：chat、dialog。"},
    "default_value": ["chat", "dialog"]
  },
  "initial_tab": {
    "help_tip": {"en": "Tab shown first; leave out for the first tab.", "zh-cn": "首先显示的标签页；留空则为第一个。"}
  },
  "width": {
    "help_tip": {"en": "Width of the panel.", "zh-cn": "面板宽度。"},
    "default_value": "elastic"
  },
  "height": {
    "help_tip": {"en": "Height of the panel; the conversation scrolls inside it.", "zh-cn": "面板高度；对话在其中滚动。"},
    "default_value": "640px"
  },
  "min_width": {
    "help_tip": {"en": "Narrowest width of the panel.", "zh-cn": "面板的最小宽度。"},
    "default_value": "280px"
  }
}$wpi$::jsonb, 'active', 'seed:wpi-chat-elements', 'seed:wpi-chat-elements'
WHERE NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE kind = 'element' AND descriptor_id = 'chat-panel');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM kb.wpi_descriptors WHERE kind = 'element' AND created_by = 'seed:wpi-chat-elements';
DELETE FROM kb.code_modules WHERE code_module_id IN ('chat-composer', 'chat-panel') AND creator = 'migration:wpi-chat-elements';
-- +goose StatementEnd
