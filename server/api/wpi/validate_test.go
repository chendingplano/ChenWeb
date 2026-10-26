package wpi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	d := json.NewDecoder(bytes.NewReader([]byte(s)))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		t.Fatalf("bad test JSON: %v\n%s", err, s)
	}
	return m
}

// expectIssue fails unless exactly one issue mentions want (or none when want is "").
func expectIssue(t *testing.T, issues []Issue, want string) {
	t.Helper()
	if want == "" {
		if len(issues) != 0 {
			t.Fatalf("unexpected issues: %+v", issues)
		}
		return
	}
	for _, is := range issues {
		if strings.Contains(is.Path+" "+is.Message, want) {
			return
		}
	}
	t.Fatalf("no issue mentions %q; got %+v", want, issues)
}

const validApp = `{"descriptor_id": "customer-admin", "schema_version": "0.1",
 "layout_template": "top-left-middle-right-bottom",
 "panes": {"top": {"height": "48px", "page": "app-header"},
           "left": {"width": "240px", "min_width": "160px", "resizable": true, "page": "customer-nav"},
           "middle": {"width": "elastic"}}}`

const validFormPage = `{"descriptor_id": "customer-form", "schema_version": "0.1",
 "layouter": "flow-layouter", "input": {}, "save_action": "save-customer",
 "elements": [
   {"element": "customer-email", "binding_mode": "read-only"},
   {"descriptor_id": "save", "element_type": "button", "label": "Save", "on_click": {"action": "save-customer"}},
   {"descriptor_id": "close", "element_type": "button", "label": "Close", "on_click": {"action_type": "close_page"}}
 ]}`

func TestValidateApp(t *testing.T) {
	expectIssue(t, ValidateDescriptor(KindApp, "customer-admin", decode(t, validApp)), "")
	cases := []struct{ from, to, want string }{
		{`"layout_template": "top-left-middle-right-bottom"`, `"layout_template": "grid-9"`, `"grid-9" is not supported`},
		{`"middle": {"width": "elastic"}`, `"side": {"width": "elastic"}`, `pane "side" is not part of layout template`},
		{`"resizable": true`, `"resizable": "yes"`, "must be true or false"},
		{`"height": "48px"`, `"height": "tall"`, `"tall" must match`},
		{`"page": "app-header"`, `"page": "app-header", "color": "red"`, `unknown attribute "color"`},
	}
	for _, c := range cases {
		body := strings.Replace(validApp, c.from, c.to, 1)
		expectIssue(t, ValidateDescriptor(KindApp, "customer-admin", decode(t, body)), c.want)
	}
}

func TestValidateCommon(t *testing.T) {
	body := decode(t, validFormPage)
	expectIssue(t, ValidateDescriptor(KindPage, "customer-form", body), "")
	expectIssue(t, ValidateDescriptor(KindPage, "other-id", body), `descriptor_id "customer-form" does not match "other-id"`)
	expectIssue(t, ValidateDescriptor(KindPage, "Bad_ID", body), "descriptor ID")

	body["schema_version"] = "0.2"
	expectIssue(t, ValidateDescriptor(KindPage, "customer-form", body), `schema_version "0.2" is not supported`)
}

func TestValidatePage(t *testing.T) {
	cases := []struct{ from, to, want string }{
		{`"layouter": "flow-layouter"`, `"layouter": "grid"`, `"grid" is not supported`},
		{`"input": {}`, `"input": {"action": "load-x", "args": {"id": "$input_doc.id"}}`, "$input_doc is not allowed"},
		{`"save_action": "save-customer"`, `"save_action": "save-customer", "color": "red"`, `unknown attribute "color"`},
		{`"save_action": "save-customer"`, `"save_action": "save-customer", "refresh": {"every_seconds": 5}`, `unknown attribute "every_seconds"`},
		{`"binding_mode": "read-only"}`, `"element_type": "text"}`, `attribute "element_type" cannot be overridden`},
		{`"descriptor_id": "close"`, `"descriptor_id": "save"`, `inline element ID "save" is used twice`},
	}
	for _, c := range cases {
		body := strings.Replace(validFormPage, c.from, c.to, 1)
		expectIssue(t, ValidateDescriptor(KindPage, "customer-form", decode(t, body)), c.want)
	}
}

func TestValidateElements(t *testing.T) {
	cases := []struct{ body, want string }{
		{`{"element_type": "input", "label": "Email", "data_binding": "$input_doc.email", "binding_mode": "read-write", "width": "elastic", "min_width": "200px"}`, ""},
		{`{"element_type": "text", "text": "Customers"}`, ""},
		{`{"element_type": "text", "text": "x", "data_binding": "$input_doc.x"}`, "exactly one of"},
		{`{"element_type": "slider"}`, `element type "slider" is not supported`},
		{`{"element_type": "input", "label": "x", "data_binding": "$input_doc.__proto__.x", "binding_mode": "read-write"}`, "is not allowed"},
		{`{"element_type": "input", "label": "x", "data_binding": "$record.x", "binding_mode": "read-write"}`, "must be a $input_doc path"},
		{`{"element_type": "input", "label": "x", "data_binding": "$input_doc.x", "binding_mode": "write"}`, `"write" is not supported`},
		{`{"element_type": "input", "label": "x", "data_binding": "$input_doc.x"}`, `missing attribute "binding_mode"`},
		{`{"element_type": "select", "label": "x", "data_binding": "$input_doc.x", "binding_mode": "read-write", "options": []}`, "must not be empty"},
		{`{"element_type": "button", "label": "Go", "on_click": {"action": "x", "args": {"id": "$record.id"}}}`, "$record is not allowed"},
		{`{"element_type": "list", "source": {"action": "load-customers"}, "presentation": "rows", "columns": [{"field": "email", "label": "Email"}], "on_row_click": {"action": "open-customer", "args": {"id": "$record.id"}}}`, ""},
		{`{"element_type": "list", "source": {"action": "load-customers"}, "presentation": "cards", "columns": [{"field": "email", "label": "Email"}]}`, `"cards" is not supported`},
		{`{"element_type": "menu", "initial_item": "a", "items": [{"item_id": "a", "label": "A", "on_click": {"action_type": "open_page", "page": "p", "pane": "middle"}}, {"item_id": "f", "label": "F", "children": [{"item_id": "b", "label": "B", "on_click": {"action": "open-b"}}]}]}`, ""},
		{`{"element_type": "menu", "initial_item": "f", "items": [{"item_id": "f", "label": "F", "children": [{"item_id": "b", "label": "B", "on_click": {"action": "x"}}]}]}`, `initial_item "f" is not a menu item that opens a page`},
		{`{"element_type": "menu", "items": [{"item_id": "a", "label": "A"}]}`, "exactly one of"},
		{`{"element_type": "menu", "items": [{"item_id": "a", "label": "A", "on_click": {"action": "x"}}, {"item_id": "a", "label": "B", "on_click": {"action": "y"}}]}`, `menu item ID "a" is used twice`},
	}
	for _, c := range cases {
		body := decode(t, c.body)
		body["descriptor_id"] = "el"
		body["schema_version"] = SchemaVersion
		expectIssue(t, ValidateDescriptor(KindElement, "el", body), c.want)
	}
}

func TestValidateActions(t *testing.T) {
	cases := []struct{ body, want string }{
		{`{"action_type": "load", "table": "kb.wpi_demo_customers", "result": "record", "where": {"id": "$args.id"}, "fields": [{"column": "id", "attribute": "id"}]}`, ""},
		{`{"action_type": "load", "table": "kb.customers", "result": "list", "fields": [{"column": "id", "attribute": "id"}], "order_by": [{"column": "user_name", "direction": "asc"}], "limit": 50}`, ""},
		{`{"action_type": "load", "table": "customers; drop", "result": "list", "fields": [{"column": "id", "attribute": "id"}]}`, "must match"},
		{`{"action_type": "load", "table": "c", "result": "list", "fields": [{"column": "id", "attribute": "id"}], "where": {"id": "$input_doc.id"}}`, "$input_doc is not allowed"},
		{`{"action_type": "load", "table": "c", "result": "list", "fields": [{"column": "id", "attribute": "id"}], "limit": 0}`, "must be a positive integer"},
		{`{"action_type": "load", "table": "c", "result": "list", "fields": [{"column": "id", "attribute": "id"}], "order_by": [{"column": "id", "direction": "up"}]}`, `"up" is not supported`},
		{`{"action_type": "save", "table": "c", "mode": "upsert", "key": "id", "fields": [{"attribute": "email", "column": "email", "required": true, "rules": {"max_length": 64, "pattern": "^.+$", "format": "email"}}]}`, ""},
		{`{"action_type": "save", "table": "c", "mode": "merge", "key": "id", "fields": [{"attribute": "email", "column": "email"}]}`, `"merge" is not supported`},
		{`{"action_type": "save", "table": "c", "mode": "insert", "key": "id", "fields": [{"attribute": "email", "column": "email", "rules": {"pattern": "("}}]}`, "invalid pattern"},
		{`{"action_type": "save", "table": "c", "mode": "insert", "key": "id", "fields": []}`, "must not be empty"},
		{`{"action_type": "open_page", "page": "customer-form", "pane": "right", "input": {"action": "load-customer", "args": {"id": "$args.id"}}}`, ""},
		{`{"action_type": "open_page", "page": "customer-form", "pane": "right", "input": {"action": "load-customer", "args": {"id": "$record.id"}}}`, "$record is not allowed"},
		{`{"action_type": "close_page"}`, ""},
		{`{"action_type": "close_page", "page": "x"}`, `unknown attribute "page"`},
		{`{"action_type": "navigate"}`, `action type "navigate" is not supported`},
	}
	for _, c := range cases {
		body := decode(t, c.body)
		body["descriptor_id"] = "act"
		body["schema_version"] = SchemaVersion
		expectIssue(t, ValidateDescriptor(KindAction, "act", body), c.want)
	}
}
