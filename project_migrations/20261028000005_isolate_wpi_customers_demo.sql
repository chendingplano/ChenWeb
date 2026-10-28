-- Give the standalone demo its own page identity. The customer-admin page
-- named customers contains actions targeting panes absent from single-pane.
-- +goose Up
-- +goose StatementBegin
INSERT INTO kb.frontend_pages (descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT 'demo-customers', COALESCE(MAX(version), 0) + 1, '0.2', $wpi${
  "descriptor_id": "demo-customers",
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
}$wpi$::jsonb, 'active', 'seed:wpi-customers-isolated', 'seed:wpi-customers-isolated'
FROM kb.frontend_pages WHERE descriptor_id = 'demo-customers'
HAVING NOT EXISTS (SELECT 1 FROM kb.frontend_pages WHERE created_by = 'seed:wpi-customers-isolated');
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by)
SELECT 'app', 'customers-demo', COALESCE(MAX(version), 0) + 1, '0.2', $wpi${
  "descriptor_id": "customers-demo",
  "schema_version": "0.2",
  "layout_template": "single-pane",
  "panes": { "main": { "width": "elastic", "page": "demo-customers" } }
}$wpi$::jsonb, 'active', 'seed:wpi-customers-isolated', 'seed:wpi-customers-isolated'
FROM kb.wpi_descriptors WHERE kind = 'app' AND descriptor_id = 'customers-demo'
HAVING NOT EXISTS (SELECT 1 FROM kb.wpi_descriptors WHERE created_by = 'seed:wpi-customers-isolated');
-- +goose StatementEnd

-- Restore the customer-admin page if the original demo seed is still active.
-- Preserve operator-created versions and all existing descriptor bodies.
UPDATE kb.frontend_pages SET status = 'retired', updated_by = 'seed:wpi-customers-isolated', updated_at = NOW()
WHERE descriptor_id = 'customers' AND created_by = 'seed:wpi-customers-demo' AND status = 'active';

-- +goose Down
UPDATE kb.frontend_pages SET status = 'active', updated_by = 'seed:wpi-customers-demo', updated_at = NOW()
WHERE descriptor_id = 'customers' AND created_by = 'seed:wpi-customers-demo'
  AND status = 'retired' AND updated_by = 'seed:wpi-customers-isolated';
DELETE FROM kb.wpi_descriptors WHERE created_by = 'seed:wpi-customers-isolated';
DELETE FROM kb.frontend_pages WHERE created_by = 'seed:wpi-customers-isolated';
