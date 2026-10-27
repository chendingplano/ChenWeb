-- WPI pages demo (OpenSpec change wpi-pages; devdoc 2026102601 §3.2). Seeds the
-- query and load-file action classes, two stored statements and a stored file,
-- and new versions of the customer-admin pages: every page gets a name and a
-- description; app-header shows its title from static data; the menu gains a
-- customer directory (a list from a stored statement, whose row click opens a
-- customer through another, with a parameter) and an About page (a stored file,
-- read-only page-wide, centred). server/api/wpi/seed_test.go validates these
-- bodies against the classes and app of 20261027000002.

-- +goose Up
-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by) VALUES
('action', 'query', 1, '0.2', $wpi${
  "descriptor_id": "query", "schema_version": "0.2",
  "description": {"en": "Runs a SQL statement stored on the server, named by its ID", "zh-cn": "运行存储在服务器上、按 ID 命名的 SQL 语句"},
  "sql_stmt_id": null, "result": null
}$wpi$::jsonb, 'active', 'seed:wpi-pages', 'seed:wpi-pages'),

('action', 'load-file', 1, '0.2', $wpi${
  "descriptor_id": "load-file", "schema_version": "0.2",
  "description": {"en": "Reads a JSON file stored on the server, named by its ID", "zh-cn": "读取存储在服务器上、按 ID 命名的 JSON 文件"},
  "file_id": null
}$wpi$::jsonb, 'active', 'seed:wpi-pages', 'seed:wpi-pages')
ON CONFLICT (kind, descriptor_id, version) DO NOTHING;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.stored_stmts (stmt_name, sql_stmt, description, creator, access_controls) VALUES
('wpi-demo-customer-directory', $sql$
SELECT id, user_name, email, phone_number
  FROM kb.wpi_demo_customers
 ORDER BY lower(user_name), id
$sql$, 'Every demo customer, by name (WPI customer-admin directory)', 'seed:wpi-pages', '{all}'),

('wpi-demo-customer-by-id', $sql$
SELECT id, user_name, phone_number, email
  FROM kb.wpi_demo_customers
 WHERE id = $1
$sql$, 'One demo customer by id ($1)', 'seed:wpi-pages', NULL)
ON CONFLICT (stmt_name) DO NOTHING;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.stored_files (file_name, file_content, description, creator, access_controls) VALUES
('wpi-demo-about', $file${
  "title": "Customer admin",
  "format": "WPI descriptor format 0.2",
  "summary": "Every page of this app is a descriptor stored in the database."
}$file$::jsonb, 'Text of the customer-admin About page', 'seed:wpi-pages', NULL)
ON CONFLICT (file_name) DO NOTHING;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.frontend_pages (descriptor_id, version, schema_version, body, status, created_by, updated_by) VALUES
('app-header', 3, '0.2', $wpi${
  "descriptor_id": "app-header", "schema_version": "0.2", "layouter": "flow-layouter",
  "page_name": {"en": "App header", "zh-cn": "应用页眉"},
  "description": {"en": "Title bar of the customer admin demo, from static data", "zh-cn": "客户管理演示的标题栏，来自静态数据"},
  "input": {"title": "Customer admin"},
  "elements": [{"element": "text", "data_binding": "$input_doc.title"}]
}$wpi$::jsonb, 'active', 'seed:wpi-pages', 'seed:wpi-pages'),

('customer-nav', 3, '0.2', $wpi${
  "descriptor_id": "customer-nav", "schema_version": "0.2", "layouter": "flow-layouter",
  "page_name": {"en": "Customer admin menu", "zh-cn": "客户管理菜单"},
  "description": {"en": "Menu of the customer admin demo", "zh-cn": "客户管理演示的菜单"},
  "elements": [{
    "element": "menu", "initial_item": "customers",
    "items": [
      {"item_id": "customer-records", "label": "Customer records",
       "children": [
         {"item_id": "customers", "label": "Customers",
          "on_click": {"action": "open-page", "page": "customers", "pane": "middle"}},
         {"item_id": "new-customer", "label": "New customer",
          "on_click": {"action": "open-page", "page": "customer-form", "pane": "middle", "input": {}}},
         {"item_id": "customer-directory", "label": "Directory",
          "on_click": {"action": "open-page", "page": "customer-directory", "pane": "middle"}}
       ]},
      {"item_id": "about", "label": "About",
       "on_click": {"action": "open-page", "page": "customer-admin-about", "pane": "middle"}}
    ]
  }]
}$wpi$::jsonb, 'active', 'seed:wpi-pages', 'seed:wpi-pages'),

('customers', 3, '0.2', $wpi${
  "descriptor_id": "customers", "schema_version": "0.2", "layouter": "flow-layouter",
  "page_name": {"en": "Customers", "zh-cn": "客户"},
  "description": {"en": "All demo customers, read with a load action; a click opens one", "zh-cn": "用 load 操作读取的全部演示客户；点击打开一位客户"},
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
}$wpi$::jsonb, 'active', 'seed:wpi-pages', 'seed:wpi-pages'),

('customer-form', 3, '0.2', $wpi${
  "descriptor_id": "customer-form", "schema_version": "0.2", "layouter": "flow-layouter",
  "page_name": {"en": "Customer", "zh-cn": "客户"},
  "description": {"en": "Creates or edits one demo customer", "zh-cn": "新建或编辑一位演示客户"},
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
}$wpi$::jsonb, 'active', 'seed:wpi-pages', 'seed:wpi-pages'),

('customer-directory', 1, '0.2', $wpi${
  "descriptor_id": "customer-directory", "schema_version": "0.2", "layouter": "flow-layouter",
  "page_name": {"en": "Customer directory", "zh-cn": "客户名录"},
  "description": {"en": "Demo customers listed by a stored SQL statement; a click opens one through another", "zh-cn": "由存储的 SQL 语句列出的演示客户；点击后通过另一条语句打开"},
  "refresh": {"on_show": true},
  "elements": [
    {"element": "text", "text": "Customer directory", "width": "100%"},
    {
      "element": "list",
      "source": {"action": "query", "sql_stmt_id": "wpi-demo-customer-directory", "result": "list"},
      "columns": [
        {"field": "user_name", "label": "User name", "width": "elastic"},
        {"field": "email", "label": "Email", "width": "elastic"}
      ],
      "on_row_click": {
        "action": "open-page", "page": "customer-form", "pane": "right",
        "args": {"id": "$record.id"},
        "input": {"action": "query", "sql_stmt_id": "wpi-demo-customer-by-id", "result": "record", "params": ["$args.id"]}
      }
    }
  ]
}$wpi$::jsonb, 'active', 'seed:wpi-pages', 'seed:wpi-pages'),

('customer-admin-about', 1, '0.2', $wpi${
  "descriptor_id": "customer-admin-about", "schema_version": "0.2", "layouter": "flow-layouter",
  "page_name": {"en": "About", "zh-cn": "关于"},
  "description": {"en": "About the customer admin demo, from a stored file; read-only and centred", "zh-cn": "关于客户管理演示，来自存储的文件；只读并居中"},
  "page_alignment": "center", "max_width": "640px", "binding_mode": "read-only",
  "input": {"action": "load-file", "file_id": "wpi-demo-about"},
  "elements": [
    {"element": "text", "data_binding": "$input_doc.title", "width": "100%"},
    {"element": "input", "label": {"en": "Format", "zh-cn": "格式"}, "data_binding": "$input_doc.format", "width": "100%", "max_width": "640px"},
    {"element": "input", "label": {"en": "Summary", "zh-cn": "简介"}, "data_binding": "$input_doc.summary", "width": "100%", "max_width": "640px"}
  ]
}$wpi$::jsonb, 'active', 'seed:wpi-pages', 'seed:wpi-pages')
ON CONFLICT (descriptor_id, version) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
DELETE FROM kb.frontend_pages WHERE created_by = 'seed:wpi-pages';
DELETE FROM kb.stored_files WHERE creator = 'seed:wpi-pages';
DELETE FROM kb.stored_stmts WHERE creator = 'seed:wpi-pages';
DELETE FROM kb.wpi_descriptors WHERE created_by = 'seed:wpi-pages';
