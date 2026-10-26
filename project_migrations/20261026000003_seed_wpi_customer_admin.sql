-- Seed the customer-admin demo app of the Web Page Interpreter pilot (OpenSpec
-- change wpi-pilot; devdoc 2026102601 §4). Version 1 of each descriptor; later
-- edits are new versions written through PUT /api/v1/wpi/descriptors, not new
-- migrations. server/api/wpi/seed_test.go validates these bodies.

-- +goose Up
-- +goose StatementBegin
INSERT INTO kb.wpi_descriptors (kind, descriptor_id, version, schema_version, body, status, created_by, updated_by) VALUES
('app', 'customer-admin', 1, '0.1', $wpi${
  "descriptor_id": "customer-admin", "schema_version": "0.1",
  "layout_template": "top-left-middle-right-bottom",
  "panes": {
    "top": {"height": "48px", "page": "app-header"},
    "left": {"width": "240px", "min_width": "160px", "resizable": true, "page": "customer-nav"},
    "middle": {"width": "elastic", "min_width": "320px", "resizable": true},
    "right": {"width": "elastic", "min_width": "360px"}
  }
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('page', 'app-header', 1, '0.1', $wpi${
  "descriptor_id": "app-header", "schema_version": "0.1", "layouter": "flow-layouter",
  "elements": [{"descriptor_id": "title", "element_type": "text", "text": "Customer admin"}]
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('page', 'customer-nav', 1, '0.1', $wpi${
  "descriptor_id": "customer-nav", "schema_version": "0.1", "layouter": "flow-layouter",
  "elements": [{
    "descriptor_id": "nav", "element_type": "menu", "width": "100%", "initial_item": "customers",
    "items": [{
      "item_id": "customer-records", "label": "Customer records",
      "children": [
        {"item_id": "customers", "label": "Customers",
         "on_click": {"action_type": "open_page", "page": "customers", "pane": "middle"}},
        {"item_id": "new-customer", "label": "New customer",
         "on_click": {"action_type": "open_page", "page": "customer-form", "pane": "middle", "input": {}}}
      ]
    }]
  }]
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('page', 'customers', 1, '0.1', $wpi${
  "descriptor_id": "customers", "schema_version": "0.1", "layouter": "flow-layouter",
  "refresh": {"on_show": true},
  "elements": [
    {"descriptor_id": "heading", "element_type": "text", "text": "Customers", "width": "100%"},
    {"element": "customer-list"}
  ]
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('page', 'customer-form', 1, '0.1', $wpi${
  "descriptor_id": "customer-form", "schema_version": "0.1", "layouter": "flow-layouter",
  "input": {}, "save_action": "save-customer",
  "elements": [
    {"element": "customer-user-name"},
    {"element": "customer-phone-number"},
    {"element": "customer-email"},
    {"descriptor_id": "save", "element_type": "button", "label": "Save",
     "on_click": {"action": "save-customer"}},
    {"descriptor_id": "close", "element_type": "button", "label": "Close",
     "on_click": {"action_type": "close_page"}}
  ]
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('element', 'customer-list', 1, '0.1', $wpi${
  "descriptor_id": "customer-list", "schema_version": "0.1", "element_type": "list",
  "width": "100%", "source": {"action": "load-customers"}, "presentation": "rows",
  "columns": [
    {"field": "user_name", "label": "User name", "width": "elastic"},
    {"field": "email", "label": "Email", "width": "elastic"},
    {"field": "phone_number", "label": "Phone", "width": "160px"}
  ],
  "on_row_click": {"action": "open-customer", "args": {"id": "$record.id"}}
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('element', 'customer-user-name', 1, '0.1', $wpi${
  "descriptor_id": "customer-user-name", "schema_version": "0.1", "element_type": "input",
  "label": "User name", "data_binding": "$input_doc.user_name", "binding_mode": "read-write",
  "width": "elastic", "min_width": "200px", "max_width": "400px"
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('element', 'customer-phone-number', 1, '0.1', $wpi${
  "descriptor_id": "customer-phone-number", "schema_version": "0.1", "element_type": "input",
  "label": "Phone", "data_binding": "$input_doc.phone_number", "binding_mode": "read-write",
  "width": "elastic", "min_width": "200px", "max_width": "400px"
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('element', 'customer-email', 1, '0.1', $wpi${
  "descriptor_id": "customer-email", "schema_version": "0.1", "element_type": "input",
  "label": "Email", "input_type": "email", "data_binding": "$input_doc.email", "binding_mode": "read-write",
  "width": "elastic", "min_width": "200px", "max_width": "400px"
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('action', 'load-customers', 1, '0.1', $wpi${
  "descriptor_id": "load-customers", "schema_version": "0.1", "action_type": "load",
  "table": "kb.wpi_demo_customers", "result": "list",
  "fields": [
    {"column": "id", "attribute": "id"},
    {"column": "user_name", "attribute": "user_name"},
    {"column": "phone_number", "attribute": "phone_number"},
    {"column": "email", "attribute": "email"}
  ],
  "order_by": [{"column": "user_name", "direction": "asc"}], "limit": 200
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('action', 'load-customer', 1, '0.1', $wpi${
  "descriptor_id": "load-customer", "schema_version": "0.1", "action_type": "load",
  "table": "kb.wpi_demo_customers", "result": "record", "where": {"id": "$args.id"},
  "fields": [
    {"column": "id", "attribute": "id"},
    {"column": "user_name", "attribute": "user_name"},
    {"column": "phone_number", "attribute": "phone_number"},
    {"column": "email", "attribute": "email"}
  ]
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('action', 'save-customer', 1, '0.1', $wpi${
  "descriptor_id": "save-customer", "schema_version": "0.1", "action_type": "save",
  "table": "kb.wpi_demo_customers", "mode": "upsert", "key": "id",
  "fields": [
    {"attribute": "user_name", "column": "user_name", "required": true, "rules": {"max_length": 64}},
    {"attribute": "phone_number", "column": "phone_number", "rules": {"pattern": "^[0-9+ -]{6,20}$"}},
    {"attribute": "email", "column": "email", "required": true, "rules": {"format": "email"}}
  ]
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot'),

('action', 'open-customer', 1, '0.1', $wpi${
  "descriptor_id": "open-customer", "schema_version": "0.1", "action_type": "open_page",
  "page": "customer-form", "pane": "right",
  "input": {"action": "load-customer", "args": {"id": "$args.id"}}
}$wpi$::jsonb, 'active', 'seed:wpi-pilot', 'seed:wpi-pilot')
ON CONFLICT (kind, descriptor_id, version) DO NOTHING;
-- +goose StatementEnd

-- +goose Down
DELETE FROM kb.wpi_descriptors WHERE version = 1 AND created_by = 'seed:wpi-pilot';
