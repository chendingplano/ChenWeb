-- WPI classes and instances (OpenSpec change wpi-classes-and-instances; devdoc
-- 2026102601 §3.5, §5). Seeds the element and action classes, rewrites the
-- customer-admin demo app in descriptor format 0.2 as version 2, and retires the
-- pilot's stored elements and actions, which are not classes. A class row holds
-- a default, or null for "every instance must set it", for instance attributes;
-- attributes it leaves out are optional. server/api/wpi/seed_test.go validates
-- these bodies.

-- +goose Up
-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by) VALUES
('element', 'text', 1, '0.2', $wpi${
  "descriptor_id": "text", "schema_version": "0.2",
  "description": {"en": "Static text, or the value of a bound attribute", "zh-cn": "静态文本，或绑定属性的值"}
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('element', 'input', 1, '0.2', $wpi${
  "descriptor_id": "input", "schema_version": "0.2",
  "description": {"en": "A one-line field for typing text", "zh-cn": "单行文本输入框"},
  "label": null, "data_binding": null, "binding_mode": "read-write", "input_type": "text",
  "width": "elastic", "min_width": "200px", "max_width": "400px"
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('element', 'select', 1, '0.2', $wpi${
  "descriptor_id": "select", "schema_version": "0.2",
  "description": {"en": "A pulldown menu of fixed options", "zh-cn": "固定选项的下拉菜单"},
  "label": null, "data_binding": null, "binding_mode": "read-write", "options": null,
  "width": "elastic", "min_width": "200px", "max_width": "400px"
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('element', 'button', 1, '0.2', $wpi${
  "descriptor_id": "button", "schema_version": "0.2",
  "description": {"en": "A button that runs an action", "zh-cn": "执行操作的按钮"},
  "label": null, "on_click": null
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('element', 'list', 1, '0.2', $wpi${
  "descriptor_id": "list", "schema_version": "0.2",
  "description": {"en": "Records from a query, one per row", "zh-cn": "查询结果列表，每行一条记录"},
  "source": null, "presentation": "rows", "columns": null, "width": "100%"
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('element', 'menu', 1, '0.2', $wpi${
  "descriptor_id": "menu", "schema_version": "0.2",
  "description": {"en": "A tree of items and folders; items open pages", "zh-cn": "由菜单项和文件夹组成的树，菜单项打开页面"},
  "items": null, "width": "100%"
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('action', 'load', 1, '0.2', $wpi${
  "descriptor_id": "load", "schema_version": "0.2",
  "description": {"en": "Reads one record or a list of records from a table", "zh-cn": "从表中读取一条或多条记录"},
  "table": null, "result": null, "fields": null
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('action', 'save', 1, '0.2', $wpi${
  "descriptor_id": "save", "schema_version": "0.2",
  "description": {"en": "Validates the input doc on the server and writes it to a table", "zh-cn": "在服务器端校验输入文档并写入表中"},
  "table": null, "mode": "upsert", "key": "id", "fields": null
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('action', 'open-page', 1, '0.2', $wpi${
  "descriptor_id": "open-page", "schema_version": "0.2",
  "description": {"en": "Opens a page in a pane", "zh-cn": "在窗格中打开页面"},
  "page": null, "pane": null
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('action', 'close-page', 1, '0.2', $wpi${
  "descriptor_id": "close-page", "schema_version": "0.2",
  "description": {"en": "Closes the page that triggered it, after the unsaved-changes check", "zh-cn": "检查未保存的更改后关闭触发它的页面"}
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('action', 'save-page', 1, '0.2', $wpi${
  "descriptor_id": "save-page", "schema_version": "0.2",
  "description": {"en": "Saves the page that triggered it with the page's save action", "zh-cn": "用页面的保存操作保存触发它的页面"}
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('app', 'customer-admin', 2, '0.2', $wpi${
  "descriptor_id": "customer-admin", "schema_version": "0.2",
  "layout_template": "top-left-middle-right-bottom",
  "panes": {
    "top": {"height": "48px", "page": "app-header"},
    "left": {"width": "240px", "min_width": "160px", "resizable": true, "page": "customer-nav"},
    "middle": {"width": "elastic", "min_width": "320px", "resizable": true},
    "right": {"width": "elastic", "min_width": "360px"}
  }
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('page', 'app-header', 2, '0.2', $wpi${
  "descriptor_id": "app-header", "schema_version": "0.2", "layouter": "flow-layouter",
  "elements": [{"element": "text", "text": "Customer admin"}]
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('page', 'customer-nav', 2, '0.2', $wpi${
  "descriptor_id": "customer-nav", "schema_version": "0.2", "layouter": "flow-layouter",
  "elements": [{
    "element": "menu", "initial_item": "customers",
    "items": [{
      "item_id": "customer-records", "label": "Customer records",
      "children": [
        {"item_id": "customers", "label": "Customers",
         "on_click": {"action": "open-page", "page": "customers", "pane": "middle"}},
        {"item_id": "new-customer", "label": "New customer",
         "on_click": {"action": "open-page", "page": "customer-form", "pane": "middle", "input": {}}}
      ]
    }]
  }]
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('page', 'customers', 2, '0.2', $wpi${
  "descriptor_id": "customers", "schema_version": "0.2", "layouter": "flow-layouter",
  "refresh": {"on_show": true},
  "elements": [
    {"element": "text", "text": "Customers", "width": "100%"},
    {
      "element": "list",
      "source": {
        "action": "load", "table": "kb.wpi_demo_customers", "result": "list",
        "fields": [
          {"column": "id", "attribute": "id"},
          {"column": "user_name", "attribute": "user_name"},
          {"column": "phone_number", "attribute": "phone_number"},
          {"column": "email", "attribute": "email"}
        ],
        "order_by": [{"column": "user_name", "direction": "asc"}], "limit": 200
      },
      "columns": [
        {"field": "user_name", "label": "User name", "width": "elastic"},
        {"field": "email", "label": "Email", "width": "elastic"},
        {"field": "phone_number", "label": "Phone", "width": "160px"}
      ],
      "on_row_click": {
        "action": "open-page", "page": "customer-form", "pane": "right",
        "args": {"id": "$record.id"},
        "input": {
          "action": "load", "table": "kb.wpi_demo_customers", "result": "record",
          "where": {"id": "$args.id"},
          "fields": [
            {"column": "id", "attribute": "id"},
            {"column": "user_name", "attribute": "user_name"},
            {"column": "phone_number", "attribute": "phone_number"},
            {"column": "email", "attribute": "email"}
          ]
        }
      }
    }
  ]
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes'),

('page', 'customer-form', 2, '0.2', $wpi${
  "descriptor_id": "customer-form", "schema_version": "0.2", "layouter": "flow-layouter",
  "input": {},
  "save_action": {
    "action": "save", "table": "kb.wpi_demo_customers",
    "fields": [
      {"attribute": "user_name", "column": "user_name", "required": true, "rules": {"max_length": 64}},
      {"attribute": "phone_number", "column": "phone_number", "rules": {"pattern": "^[0-9+ -]{6,20}$"}},
      {"attribute": "email", "column": "email", "required": true, "rules": {"format": "email"}}
    ]
  },
  "elements": [
    {"element": "input", "label": {"en": "User name", "zh-cn": "用户名"}, "data_binding": "$input_doc.user_name"},
    {"element": "input", "label": {"en": "Phone", "zh-cn": "电话"}, "data_binding": "$input_doc.phone_number"},
    {"element": "input", "label": {"en": "Email", "zh-cn": "电子邮箱"}, "input_type": "email", "data_binding": "$input_doc.email"},
    {"element": "button", "label": {"en": "Save", "zh-cn": "保存"}, "on_click": {"action": "save-page"}},
    {"element": "button", "label": {"en": "Close", "zh-cn": "关闭"}, "on_click": {"action": "close-page"}}
  ]
}$wpi$::jsonb, 'active', 'seed:wpi-classes', 'seed:wpi-classes')
ON CONFLICT (kind, descriptor_id, version) DO NOTHING;
-- +goose StatementEnd

-- The pilot's stored elements and actions are single uses, not classes.
UPDATE kb.wpi_descriptors
   SET status = 'retired', updated_by = 'seed:wpi-classes', updated_at = NOW()
 WHERE created_by = 'seed:wpi-pilot' AND kind IN ('element', 'action') AND status = 'active';

-- +goose Down
UPDATE kb.wpi_descriptors
   SET status = 'active', updated_by = 'seed:wpi-pilot', updated_at = NOW()
 WHERE created_by = 'seed:wpi-pilot' AND kind IN ('element', 'action')
   AND status = 'retired' AND updated_by = 'seed:wpi-classes';

DELETE FROM kb.wpi_descriptors WHERE created_by = 'seed:wpi-classes';
