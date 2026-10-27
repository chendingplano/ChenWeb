-- WPI element class rows in format 0.3 (OpenSpec change wpi-create-element-class;
-- devdoc 2026102601 §3.5.1). Each instance attribute is now an object with an
-- optional help_tip for authors and an optional default_value (null: every
-- instance must set it; left out: optional, no default). Stores version 2 of the
-- six element classes seeded by 20261027000002, with the same defaults and a
-- help tip for every attribute; the list's on_row_click and the menu's
-- initial_item, optional with no default, are now listed too. Apps, pages and
-- action classes stay in format 0.2. server/api/wpi/seed_test.go validates these
-- bodies.

-- +goose Up
-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by) VALUES
('element', 'text', 2, '0.3', $wpi${
  "descriptor_id": "text", "schema_version": "0.3",
  "description": {"en": "Static text, or the value of a bound attribute", "zh-cn": "静态文本，或绑定属性的值"},
  "text": {"help_tip": {"en": "The text to show. Set either text or data_binding, not both.", "zh-cn": "要显示的文本。text 与 data_binding 只能设置其一。"}},
  "data_binding": {"help_tip": {"en": "The input doc attribute whose value is shown, such as $input_doc.name. Set either text or data_binding, not both.", "zh-cn": "显示其值的输入文档属性，例如 $input_doc.name。text 与 data_binding 只能设置其一。"}}
}$wpi$::jsonb, 'active', 'seed:wpi-element-classes-0.3', 'seed:wpi-element-classes-0.3'),

('element', 'input', 2, '0.3', $wpi${
  "descriptor_id": "input", "schema_version": "0.3",
  "description": {"en": "A one-line field for typing text", "zh-cn": "单行文本输入框"},
  "label": {"help_tip": {"en": "The caption shown with the field.", "zh-cn": "字段旁显示的标题。"}, "default_value": null},
  "data_binding": {"help_tip": {"en": "The input doc attribute the field shows and edits, such as $input_doc.email.", "zh-cn": "字段显示并编辑的输入文档属性，例如 $input_doc.email。"}, "default_value": null},
  "binding_mode": {"help_tip": {"en": "read-write lets the user edit the value; read-only only shows it.", "zh-cn": "read-write 允许用户编辑该值；read-only 只显示该值。"}, "default_value": "read-write"},
  "input_type": {"help_tip": {"en": "The kind of value typed: text, number or email.", "zh-cn": "输入值的类型：text、number 或 email。"}, "default_value": "text"},
  "width": {"help_tip": {"en": "The field's width: a length such as 240px, elastic (shares the free space of its row) or auto.", "zh-cn": "字段宽度：长度（如 240px）、elastic（分享所在行的剩余空间）或 auto。"}, "default_value": "elastic"},
  "min_width": {"help_tip": {"en": "The smallest an elastic width may become.", "zh-cn": "elastic 宽度可缩小到的最小值。"}, "default_value": "200px"},
  "max_width": {"help_tip": {"en": "The largest an elastic width may become.", "zh-cn": "elastic 宽度可增大到的最大值。"}, "default_value": "400px"}
}$wpi$::jsonb, 'active', 'seed:wpi-element-classes-0.3', 'seed:wpi-element-classes-0.3'),

('element', 'select', 2, '0.3', $wpi${
  "descriptor_id": "select", "schema_version": "0.3",
  "description": {"en": "A pulldown menu of fixed options", "zh-cn": "固定选项的下拉菜单"},
  "label": {"help_tip": {"en": "The caption shown with the pulldown menu.", "zh-cn": "下拉菜单旁显示的标题。"}, "default_value": null},
  "data_binding": {"help_tip": {"en": "The input doc attribute that holds the chosen option's value.", "zh-cn": "保存所选选项值的输入文档属性。"}, "default_value": null},
  "binding_mode": {"help_tip": {"en": "read-write lets the user choose; read-only only shows the choice.", "zh-cn": "read-write 允许用户选择；read-only 只显示所选项。"}, "default_value": "read-write"},
  "options": {"help_tip": {"en": "The choices, in order, each with a value and a label.", "zh-cn": "按顺序排列的选项，每项包含 value 和 label。"}, "default_value": null},
  "width": {"help_tip": {"en": "The menu's width: a length such as 240px, elastic (shares the free space of its row) or auto.", "zh-cn": "菜单宽度：长度（如 240px）、elastic（分享所在行的剩余空间）或 auto。"}, "default_value": "elastic"},
  "min_width": {"help_tip": {"en": "The smallest an elastic width may become.", "zh-cn": "elastic 宽度可缩小到的最小值。"}, "default_value": "200px"},
  "max_width": {"help_tip": {"en": "The largest an elastic width may become.", "zh-cn": "elastic 宽度可增大到的最大值。"}, "default_value": "400px"}
}$wpi$::jsonb, 'active', 'seed:wpi-element-classes-0.3', 'seed:wpi-element-classes-0.3'),

('element', 'button', 2, '0.3', $wpi${
  "descriptor_id": "button", "schema_version": "0.3",
  "description": {"en": "A button that runs an action", "zh-cn": "执行操作的按钮"},
  "label": {"help_tip": {"en": "The text on the button.", "zh-cn": "按钮上的文字。"}, "default_value": null},
  "on_click": {"help_tip": {"en": "The action run when the button is clicked, such as save-page, close-page or open-page.", "zh-cn": "点击按钮时执行的动作，例如 save-page、close-page 或 open-page。"}, "default_value": null}
}$wpi$::jsonb, 'active', 'seed:wpi-element-classes-0.3', 'seed:wpi-element-classes-0.3'),

('element', 'list', 2, '0.3', $wpi${
  "descriptor_id": "list", "schema_version": "0.3",
  "description": {"en": "Records from a query, one per row", "zh-cn": "查询结果列表，每行一条记录"},
  "source": {"help_tip": {"en": "The load or query action that reads the records. It runs when the list is drawn and when its page refreshes.", "zh-cn": "读取记录的 load 或 query 动作。在列表绘制时及其页面刷新时运行。"}, "default_value": null},
  "presentation": {"help_tip": {"en": "How records look. Only rows, one record per row, is supported.", "zh-cn": "记录的展示方式。目前仅支持 rows，即每行一条记录。"}, "default_value": "rows"},
  "columns": {"help_tip": {"en": "The columns, each with the record field it shows, a label and an optional width.", "zh-cn": "各列，每列包含所显示的记录字段、标题和可选宽度。"}, "default_value": null},
  "on_row_click": {"help_tip": {"en": "The action run when a row is clicked; $record is that row's record.", "zh-cn": "点击某行时执行的动作；$record 为该行的记录。"}},
  "width": {"help_tip": {"en": "The list's width; 100% fills its row.", "zh-cn": "列表宽度；100% 占满所在行。"}, "default_value": "100%"}
}$wpi$::jsonb, 'active', 'seed:wpi-element-classes-0.3', 'seed:wpi-element-classes-0.3'),

('element', 'menu', 2, '0.3', $wpi${
  "descriptor_id": "menu", "schema_version": "0.3",
  "description": {"en": "A tree of items and folders; items open pages", "zh-cn": "由菜单项和文件夹组成的树，菜单项打开页面"},
  "items": {"help_tip": {"en": "The menu tree. Each item has an item_id and a label, and either an on_click that opens a page or children, which make it a folder.", "zh-cn": "菜单树。每个菜单项有 item_id 和 label，并且要么有打开页面的 on_click，要么有 children（使其成为文件夹）。"}, "default_value": null},
  "initial_item": {"help_tip": {"en": "The item_id of the item opened when the app starts.", "zh-cn": "应用启动时打开的菜单项的 item_id。"}},
  "width": {"help_tip": {"en": "The menu's width; 100% fills its row.", "zh-cn": "菜单宽度；100% 占满所在行。"}, "default_value": "100%"}
}$wpi$::jsonb, 'active', 'seed:wpi-element-classes-0.3', 'seed:wpi-element-classes-0.3')
ON CONFLICT (kind, descriptor_id, version) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM kb.wpi_descriptors
WHERE kind = 'element' AND version = 2 AND created_by = 'seed:wpi-element-classes-0.3';
-- +goose StatementEnd
