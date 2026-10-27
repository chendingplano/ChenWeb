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

// testClasses are class rows shaped like the seed's.
func testClasses() map[Kind]map[string]string {
	return map[Kind]map[string]string{
		KindElement: {
			"text":   `{"descriptor_id": "text", "schema_version": "0.2"}`,
			"input":  `{"descriptor_id": "input", "schema_version": "0.2", "description": {"en": "A field", "zh-cn": "输入框"}, "label": null, "data_binding": null, "binding_mode": "read-write", "width": "elastic"}`,
			"select": `{"descriptor_id": "select", "schema_version": "0.2", "label": null, "data_binding": null, "binding_mode": "read-write", "options": null}`,
			"button": `{"descriptor_id": "button", "schema_version": "0.2", "label": null, "on_click": null}`,
			"list":   `{"descriptor_id": "list", "schema_version": "0.2", "source": null, "presentation": "rows", "columns": null}`,
			"menu":   `{"descriptor_id": "menu", "schema_version": "0.2", "items": null}`,
		},
		KindAction: {
			"load":       `{"descriptor_id": "load", "schema_version": "0.2", "table": null, "result": null, "fields": null}`,
			"save":       `{"descriptor_id": "save", "schema_version": "0.2", "table": null, "mode": "upsert", "key": "id", "fields": null}`,
			"open-page":  `{"descriptor_id": "open-page", "schema_version": "0.2", "page": null, "pane": null}`,
			"close-page": `{"descriptor_id": "close-page", "schema_version": "0.2"}`,
			"save-page":  `{"descriptor_id": "save-page", "schema_version": "0.2"}`,
		},
	}
}

func classDescriptors(t *testing.T, rows map[Kind]map[string]string) map[Kind]map[string]Descriptor {
	t.Helper()
	out := map[Kind]map[string]Descriptor{}
	for kind, byID := range rows {
		out[kind] = map[string]Descriptor{}
		for id, s := range byID {
			out[kind][id] = Descriptor{Kind: kind, ID: id, Version: 1, Raw: []byte(s), Body: decode(t, s)}
		}
	}
	return out
}

const validApp = `{"descriptor_id": "customer-admin", "schema_version": "0.2",
 "layout_template": "top-left-middle-right-bottom",
 "panes": {"top": {"height": "48px", "page": "app-header"},
           "left": {"width": "240px", "min_width": "160px", "resizable": true, "page": "customer-nav"},
           "middle": {"width": "elastic"}}}`

const validFormPage = `{"descriptor_id": "customer-form", "schema_version": "0.2",
 "layouter": "flow-layouter", "input": {},
 "save_action": {"action": "save", "table": "kb.wpi_demo_customers", "fields": [{"attribute": "email", "column": "email", "required": true}]},
 "elements": [
   {"element": "input", "label": {"en": "Email", "zh-cn": "电子邮箱"}, "data_binding": "$input_doc.email", "binding_mode": "read-only"},
   {"element": "button", "label": "Save", "on_click": {"action": "save-page"}},
   {"element": "button", "label": "Close", "on_click": {"action": "close-page"}}
 ]}`

// pageWith is a page holding one element instance.
func pageWith(element string) string {
	return `{"descriptor_id": "p", "schema_version": "0.2", "layouter": "flow-layouter", "elements": [` + element + `]}`
}

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

	body["schema_version"] = "0.1"
	expectIssue(t, ValidateDescriptor(KindPage, "customer-form", body), `schema_version "0.1" is not supported`)
}

func TestValidatePage(t *testing.T) {
	cases := []struct{ from, to, want string }{
		{`"layouter": "flow-layouter"`, `"layouter": "grid"`, `"grid" is not supported`},
		{`"input": {}`, `"input": {"action": "load", "table": "c", "result": "record", "fields": [{"column": "id", "attribute": "id"}], "args": {"id": "$input_doc.id"}}`, "$input_doc is not allowed"},
		{`"input": {}`, `"input": {"action": "save-page"}`, "a save-page action is not allowed here (allowed: load)"},
		{`"input": {}`, `"input": {}, "color": "red"`, `unknown attribute "color"`},
		{`"input": {}`, `"input": {}, "refresh": {"every_seconds": 5}`, `unknown attribute "every_seconds"`},
		{`"save_action": {"action": "save"`, `"save_action": {"action": "load"`, "a load action is not allowed here (allowed: save)"},
		{`"save_action": {"action": "save", "table"`, `"save_action": {"action": "save", "descriptor_id": "x", "table"`, `"descriptor_id" is a class attribute`},
		{`{"action": "save-page"}`, `{"action": "load", "table": "c", "result": "list", "fields": [{"column": "id", "attribute": "id"}]}`, "a load action is not allowed here"},
		{`{"action": "close-page"}`, `{"action": "navigate"}`, `action class "navigate" has no code`},
		{`{"action": "close-page"}`, `{"page": "x"}`, `missing attribute "action"`},
		{`{"action": "close-page"}`, `{"action": "close-page", "page": "x"}`, `unknown attribute "page"`},
		{`"element": "input"`, `"element": "slider"`, `element class "slider" has no code`},
		{`"element": "input"`, `"description": "x", "element": "input"`, `"description" is a class attribute`},
		{`{"element": "button", "label": "Save"`, `{"label": "Save"`, `missing attribute "element"`},
		{`"binding_mode": "read-only"`, `"binding_mode": "read-only", "on_click": {"action": "close-page"}`, `unknown attribute "on_click"`},
	}
	for _, c := range cases {
		body := strings.Replace(validFormPage, c.from, c.to, 1)
		if body == validFormPage {
			t.Fatalf("case %q did not change the page", c.want)
		}
		expectIssue(t, ValidateDescriptor(KindPage, "customer-form", decode(t, body)), c.want)
	}
	pilotForm := strings.Replace(validFormPage, `"save_action": {"action": "save"`, `"save_action": "save-customer", "x": {"action": "save"`, 1)
	expectIssue(t, ValidateDescriptor(KindPage, "customer-form", decode(t, pilotForm)), "/save_action must be an object")
}

// TestValidateElementInstances covers the checks made when a page is written.
func TestValidateElementInstances(t *testing.T) {
	cases := []struct{ body, want string }{
		{`{"element": "input", "label": "Email", "data_binding": "$input_doc.email", "binding_mode": "read-write", "width": "elastic", "min_width": "200px"}`, ""},
		{`{"element": "input", "label": {"en": "Email", "zh-cn": "电子邮箱"}, "data_binding": "$input_doc.email"}`, ""},
		{`{"element": "input", "label": {"zh-cn": "电子邮箱"}, "data_binding": "$input_doc.email"}`, `need an "en" entry`},
		{`{"element": "input", "label": {"en": "Email", "EN": "x"}, "data_binding": "$input_doc.email"}`, `"EN" is not a language code`},
		{`{"element": "input", "label": "", "data_binding": "$input_doc.email"}`, "must be a non-empty string or an object"},
		{`{"element": "text", "text": "Customers"}`, ""},
		{`{"element": "text", "text": 5}`, "must be a string"},
		{`{"element": "input", "label": "x", "data_binding": "$input_doc.__proto__.x"}`, "is not allowed"},
		{`{"element": "input", "label": "x", "data_binding": "$record.x"}`, "must be a $input_doc path"},
		{`{"element": "input", "label": "x", "data_binding": "$input_doc.x", "binding_mode": "write"}`, `"write" is not supported`},
		{`{"element": "select", "label": "x", "data_binding": "$input_doc.x", "options": []}`, "must not be empty"},
		{`{"element": "button", "label": "Go", "on_click": {"action": "save-page", "args": {"id": "$record.id"}}}`, "$record is not allowed"},
		{`{"element": "list", "source": {"action": "load", "table": "c", "result": "list", "fields": [{"column": "email", "attribute": "email"}]},
		   "columns": [{"field": "email", "label": "Email"}],
		   "on_row_click": {"action": "open-page", "page": "f", "pane": "right", "args": {"id": "$record.id"}}}`, ""},
		{`{"element": "list", "presentation": "cards"}`, `"cards" is not supported`},
		{`{"element": "list", "source": {"action": "save", "table": "c", "fields": [{"attribute": "a", "column": "a"}]}}`, "a save action is not allowed here (allowed: load)"},
		{`{"element": "menu", "initial_item": "a", "items": [{"item_id": "a", "label": "A", "on_click": {"action": "open-page", "page": "p", "pane": "middle"}}, {"item_id": "f", "label": "F", "children": [{"item_id": "b", "label": "B", "on_click": {"action": "open-page", "page": "q", "pane": "middle"}}]}]}`, ""},
		{`{"element": "menu", "items": [{"item_id": "a", "label": "A", "on_click": {"action": "load"}}]}`, "a load action is not allowed here"},
		{`{"element": "menu", "items": [{"item_id": "a", "label": "A"}]}`, "exactly one of"},
		{`{"element": "menu", "items": [{"item_id": "a", "label": "A", "on_click": {"action": "close-page"}}, {"item_id": "a", "label": "B", "on_click": {"action": "close-page"}}]}`, `menu item ID "a" is used twice`},
	}
	for _, c := range cases {
		expectIssue(t, ValidateDescriptor(KindPage, "p", decode(t, pageWith(c.body))), c.want)
	}
}

// TestValidateActionInstances covers action instances in each kind of place.
func TestValidateActionInstances(t *testing.T) {
	onInput := func(a string) string {
		return `{"descriptor_id": "p", "schema_version": "0.2", "layouter": "flow-layouter", "elements": [], "input": ` + a + `}`
	}
	onSave := func(a string) string {
		return `{"descriptor_id": "p", "schema_version": "0.2", "layouter": "flow-layouter", "elements": [], "save_action": ` + a + `}`
	}
	onClick := func(a string) string { return pageWith(`{"element": "button", "label": "Go", "on_click": ` + a + `}`) }
	cases := []struct{ page, want string }{
		{onInput(`{"action": "load", "table": "kb.wpi_demo_customers", "result": "record", "where": {"id": "$args.id"}, "fields": [{"column": "id", "attribute": "id"}]}`), ""},
		{onInput(`{"action": "load", "table": "kb.customers", "result": "list", "fields": [{"column": "id", "attribute": "id"}], "order_by": [{"column": "user_name", "direction": "asc"}], "limit": 50}`), ""},
		{onInput(`{"action": "load", "table": "customers; drop", "result": "list", "fields": [{"column": "id", "attribute": "id"}]}`), "must match"},
		{onInput(`{"action": "load", "table": "c", "result": "list", "fields": [{"column": "id", "attribute": "id"}], "where": {"id": "$input_doc.id"}}`), "$input_doc is not allowed"},
		{onInput(`{"action": "load", "table": "c", "result": "list", "fields": [{"column": "id", "attribute": "id"}], "limit": 0}`), "must be a positive integer"},
		{onInput(`{"action": "load", "table": "c", "result": "list", "fields": [{"column": "id", "attribute": "id"}], "order_by": [{"column": "id", "direction": "up"}]}`), `"up" is not supported`},
		{onSave(`{"action": "save", "table": "c", "mode": "upsert", "key": "id", "fields": [{"attribute": "email", "column": "email", "required": true, "rules": {"max_length": 64, "pattern": "^.+$", "format": "email"}}]}`), ""},
		{onSave(`{"action": "save", "table": "c", "mode": "merge", "fields": [{"attribute": "email", "column": "email"}]}`), `"merge" is not supported`},
		{onSave(`{"action": "save", "table": "c", "fields": [{"attribute": "email", "column": "email", "rules": {"pattern": "("}}]}`), "invalid pattern"},
		{onSave(`{"action": "save", "table": "c", "fields": []}`), "must not be empty"},
		{onClick(`{"action": "open-page", "page": "customer-form", "pane": "right", "args": {"id": "$input_doc.id"}, "input": {"action": "load", "table": "c", "result": "record", "where": {"id": "$args.id"}, "fields": [{"column": "id", "attribute": "id"}]}}`), ""},
		{onClick(`{"action": "open-page", "page": "customer-form", "pane": "right", "input": {"action": "load", "table": "c", "result": "record", "fields": [{"column": "id", "attribute": "id"}], "args": {"id": "$record.id"}}}`), "$record is not allowed"},
		{onClick(`{"action": "open-page", "page": "customer-form", "pane": "right", "input": {"action": "save-page"}}`), "a save-page action is not allowed here (allowed: load)"},
		{onClick(`{"action": "open-page", "page": "customer-form", "pane": "right", "input": {}}`), ""},
		{onClick(`{"action": "save-page"}`), ""},
		{onClick(`{"action": "save", "table": "c", "fields": [{"attribute": "a", "column": "a"}]}`), ""},
	}
	for _, c := range cases {
		expectIssue(t, ValidateDescriptor(KindPage, "p", decode(t, c.page)), c.want)
	}
}

func TestValidateClassRows(t *testing.T) {
	for kind, rows := range testClasses() {
		for id, s := range rows {
			expectIssue(t, ValidateDescriptor(kind, id, decode(t, s)), "")
		}
	}
	cases := []struct {
		kind     Kind
		id, body string
		want     string
	}{
		{KindElement, "slider", `{}`, `element class "slider" has no code in this interpreter`},
		{KindAction, "navigate", `{}`, `action class "navigate" has no code in this interpreter`},
		{KindElement, "input", `{"label": null, "data_binding": null, "binding_mode": "read-write", "on_row_click": null}`, `unknown attribute "on_row_click"`},
		{KindElement, "input", `{"label": null, "binding_mode": "read-write"}`, `must give "data_binding" a default or null`},
		{KindElement, "input", `{"label": null, "data_binding": null, "binding_mode": "write"}`, `"write" is not supported`},
		{KindElement, "input", `{"label": null, "data_binding": null, "binding_mode": "read-write", "description": {"zh-cn": "输入框"}}`, `need an "en" entry`},
		{KindElement, "button", `{"label": "Go", "on_click": {"action": "close-page"}}`, `attribute "on_click" holds actions`},
		{KindElement, "list", `{"source": null, "columns": null, "presentation": "rows", "on_row_click": {"action": "close-page"}}`, `attribute "on_row_click" holds actions`},
		{KindAction, "open-page", `{"page": null, "pane": null, "input": {}}`, `attribute "input" holds actions`},
		{KindAction, "save", `{"table": null, "mode": "merge", "key": "id", "fields": null}`, `"merge" is not supported`},
		{KindAction, "save-page", `{"args": {}}`, `unknown attribute "args"`},
	}
	for _, c := range cases {
		body := decode(t, c.body)
		body["descriptor_id"] = c.id
		body["schema_version"] = SchemaVersion
		expectIssue(t, ValidateDescriptor(c.kind, c.id, body), c.want)
	}
}

// TestPageInstances covers the checks made against class rows when a bundle
// is built.
func TestPageInstances(t *testing.T) {
	check := func(t *testing.T, classes map[Kind]map[string]string, page string) ([]mergedElement, []mergedAction, []Issue) {
		t.Helper()
		body := decode(t, page)
		if issues := ValidateDescriptor(KindPage, str(body["descriptor_id"]), body); len(issues) > 0 {
			t.Fatalf("page must be valid when written: %+v", issues)
		}
		v := &validator{kind: KindPage, id: "p", classes: classDescriptors(t, classes)}
		els, acts := v.pageInstances(body)
		return els, acts, v.issues
	}
	t.Run("defaults merged", func(t *testing.T) {
		els, acts, issues := check(t, testClasses(), validFormPage)
		expectIssue(t, issues, "")
		if len(els) != 3 || str(els[0].body["width"]) != "elastic" || str(els[0].body["binding_mode"]) != "read-only" {
			t.Fatalf("elements %+v", els)
		}
		save := acts[0]
		if save.path != "/save_action" || str(save.body["mode"]) != "upsert" || str(save.body["key"]) != "id" {
			t.Fatalf("save %+v", save)
		}
	})
	cases := []struct {
		name, element, want string
		edit                func(map[Kind]map[string]string)
	}{
		{"null attribute not set", `{"element": "input", "label": "x"}`, `attribute "data_binding" has no default in element class "input", so this instance must set it`, nil},
		{"text needs text or binding", `{"element": "text"}`, "exactly one of", nil},
		{"text with both", `{"element": "text", "text": "x", "data_binding": "$input_doc.x"}`, "exactly one of", nil},
		{"initial item is a folder", `{"element": "menu", "initial_item": "f", "items": [{"item_id": "f", "label": "F", "children": [{"item_id": "b", "label": "B", "on_click": {"action": "close-page"}}]}]}`,
			`initial_item "f" is not a menu item that opens a page`, nil},
		{"class row missing", `{"element": "text", "text": "x"}`, `element class "text" does not exist`,
			func(c map[Kind]map[string]string) { delete(c[KindElement], "text") }},
		{"code-required attribute missing after merge", `{"element": "list", "source": {"action": "load", "table": "c", "result": "list", "fields": [{"column": "a", "attribute": "a"}]}}`,
			`missing attribute "columns"`, func(c map[Kind]map[string]string) {
				c[KindElement]["list"] = `{"descriptor_id": "list", "schema_version": "0.2", "source": null, "presentation": "rows"}`
			}},
		{"action null attribute not set", `{"element": "button", "label": "Go", "on_click": {"action": "open-page", "page": "x"}}`,
			`attribute "pane" has no default in action class "open-page"`, nil},
		{"action class row missing", `{"element": "button", "label": "Go", "on_click": {"action": "save-page"}}`,
			`action class "save-page" does not exist`, func(c map[Kind]map[string]string) { delete(c[KindAction], "save-page") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			classes := testClasses()
			if c.edit != nil {
				c.edit(classes)
			}
			_, _, issues := check(t, classes, pageWith(c.element))
			expectIssue(t, issues, c.want)
		})
	}
}

func TestActionPositions(t *testing.T) {
	page := decode(t, `{"input": {}, "save_action": {"action": "save"}, "elements": [
	  {"element": "list", "source": {"action": "load"}, "on_row_click": {"action": "open-page", "input": {"action": "load"}}},
	  {"element": "menu", "items": [{"item_id": "f", "children": [{"item_id": "a", "on_click": {"action": "close-page"}}]}]},
	  {"element": "input", "on_click": {"action": "close-page"}}]}`)
	var got []string
	for _, p := range actionPositions(page) {
		got = append(got, p.path)
	}
	want := "/input /save_action /elements/0/source /elements/0/on_row_click /elements/1/items/0/children/0/on_click /elements/0/on_row_click/input"
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
}
