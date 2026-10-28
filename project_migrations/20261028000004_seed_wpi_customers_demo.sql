-- interpreter-page.md §5, exposed under Development => Demos => Customers.
-- Keep the page ID from the example; append a version without overwriting
-- existing versions. The customer-admin demo also resolves this page ID.
-- +goose Up
-- +goose StatementBegin
INSERT INTO kb.frontend_pages (descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT 'customers', COALESCE(MAX(version), 0) + 1, '0.2', $wpi${
  "descriptor_id": "customers",
  "schema_version": "0.2",
  "page_name": { "en": "Customers", "zh-cn": "客户" },
  "description": "Lists demo customers with their phone numbers and email addresses",
  "layouter": "flow-layouter",
  "refresh": { "on_show": true },
  "elements": [
    {
      "element": "text",
      "text": "Demo - Customers",
      "width": "100%"
    },
    {
      "element": "list",
      "width": "100%",
      "source": {
        "action": "load",
        "table": "kb.wpi_demo_customers",
        "result": "list",
        "fields": [
          { "column": "user_name", "attribute": "user_name" },
          { "column": "phone_number", "attribute": "phone_number" },
          { "column": "email", "attribute": "email" }
        ],
        "order_by": [ { "column": "user_name", "direction": "asc" } ],
        "limit": 200
      },
      "presentation": "rows",
      "columns": [
        { "field": "user_name", "label": "User name", "width": "elastic" },
        { "field": "phone_number", "label": "Phone number", "width": "160px" },
        { "field": "email", "label": "Email", "width": "elastic" }
      ]
    }
  ]
}$wpi$::jsonb, 'active', 'seed:wpi-customers-demo', 'seed:wpi-customers-demo'
FROM kb.frontend_pages WHERE descriptor_id = 'customers'
HAVING NOT EXISTS (SELECT 1 FROM kb.frontend_pages WHERE created_by = 'seed:wpi-customers-demo');
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by) VALUES
('app', 'customers-demo', 1, '0.2', $wpi${
  "descriptor_id": "customers-demo",
  "schema_version": "0.2",
  "layout_template": "single-pane",
  "panes": { "main": { "width": "elastic", "page": "customers" } }
}$wpi$::jsonb, 'active', 'seed:wpi-customers-demo', 'seed:wpi-customers-demo')
ON CONFLICT (kind, descriptor_id, version) DO NOTHING;
-- +goose StatementEnd

INSERT INTO kb.page_config (page_key, entry_key, language, content, entry_desc, created_by, updated_by)
SELECT 'development', 'dev-demos-customers', language, '{}'::jsonb,
       '/development => Development => Demos => Customers',
       'seed:wpi-customers-demo', 'seed:wpi-customers-demo'
FROM (VALUES ('en'), ('zh-cn')) AS languages(language)
ON CONFLICT (page_key, entry_key, language) DO NOTHING;

UPDATE kb.page_config pc SET access_role = parent.access_role
FROM kb.page_config parent
WHERE pc.page_key = 'development' AND pc.entry_key = 'dev-demos-customers'
  AND parent.page_key = 'development' AND parent.entry_key = 'dev-demos'
  AND parent.language = 'zh-cn' AND pc.access_role IS NULL;

-- +goose Down
DELETE FROM kb.page_config WHERE page_key = 'development' AND created_by = 'seed:wpi-customers-demo';
DELETE FROM kb.wpi_descriptors WHERE created_by = 'seed:wpi-customers-demo';
DELETE FROM kb.frontend_pages WHERE created_by = 'seed:wpi-customers-demo';
