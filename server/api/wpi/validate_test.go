package wpi

import (
	"bytes"
	"context"
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

func TestListPaginationAndPixelSizes(t *testing.T) {
	for _, tc := range []struct{ attrs, want string }{
		{`"pagination": "yes", "width": 600, "height": 300`, ""},
		{`"pagination": "no", "width": "elastic", "height": "auto"`, ""},
		{`"width": 0, "height": 0`, ""},
		{`"pagination": true`, "/pagination"},
		{`"pagination": "yet"`, "/pagination"},
		{`"width": -1`, "/width"},
		{`"height": 2.5`, "/height"},
	} {
		t.Run(tc.attrs, func(t *testing.T) {
			v := &validator{kind: KindElement, id: "list"}
			v.element("list", decode(t, `{`+tc.attrs+`}`), "")
			expectIssue(t, v.issues, tc.want)
		})
	}
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

// testClasses are class rows shaped like the seed's, and the layouter classes.
func testClasses() map[Kind]map[string]string {
	return map[Kind]map[string]string{
		KindElement: {
			"text":   `{"descriptor_id": "text", "schema_version": "0.4", "code_module_id": "text", "text": {"help_tip": "The text to show"}, "data_binding": {}}`,
			"input":  `{"descriptor_id": "input", "schema_version": "0.4", "code_module_id": "input", "description": {"en": "A field", "zh-cn": "输入框"}, "implement_notes": "A one-line field", "label": {"help_tip": {"en": "Caption", "zh-cn": "标题"}, "default_value": null}, "data_binding": {"default_value": null}, "binding_mode": {"default_value": "read-write"}, "input_type": {"default_value": "text"}, "width": {"default_value": "elastic"}}`,
			"select": `{"descriptor_id": "select", "schema_version": "0.4", "code_module_id": "select", "label": {"default_value": null}, "data_binding": {"default_value": null}, "binding_mode": {"default_value": "read-write"}, "options": {"default_value": null}}`,
			"button": `{"descriptor_id": "button", "schema_version": "0.4", "code_module_id": "button", "label": {"default_value": null}, "on_click": {"default_value": null}}`,
			"list":   `{"descriptor_id": "list", "schema_version": "0.4", "code_module_id": "list", "source": {"default_value": null}, "presentation": {"default_value": "rows"}, "columns": {"default_value": null}, "on_row_click": {"help_tip": "Runs when a row is clicked"}}`,
			"menu":   `{"descriptor_id": "menu", "schema_version": "0.4", "code_module_id": "menu", "items": {"default_value": null}, "initial_item": {}}`,
		},
		KindAction: {
			"load":       `{"descriptor_id": "load", "schema_version": "0.2", "table": null, "result": null, "fields": null}`,
			"save":       `{"descriptor_id": "save", "schema_version": "0.2", "table": null, "mode": "upsert", "key": "id", "fields": null}`,
			"open-page":  `{"descriptor_id": "open-page", "schema_version": "0.2", "page": null, "pane": null}`,
			"close-page": `{"descriptor_id": "close-page", "schema_version": "0.2"}`,
			"save-page":  `{"descriptor_id": "save-page", "schema_version": "0.2"}`,
			"query":      `{"descriptor_id": "query", "schema_version": "0.2", "sql_stmt_id": null, "result": null}`,
			"load-file":  `{"descriptor_id": "load-file", "schema_version": "0.2", "file_id": null}`,
		},
		KindPageLayouter: {
			"single-pane": `{"descriptor_id": "single-pane", "schema_version": "0.5", "description": "One pane", "code_module_id": "page-layouter", "root": {"pane_id": "main"}}`,
			"top-left-middle-right-bottom": `{"descriptor_id": "top-left-middle-right-bottom", "schema_version": "0.5", "code_module_id": "page-layouter",
			  "root": {"pane_id": "page", "split": "rows", "children": [{"pane_id": "top", "height": "48px"},
			    {"pane_id": "middle-row", "split": "columns", "children": [{"pane_id": "left", "width": "240px", "resizable": true}, {"pane_id": "middle"}, {"pane_id": "right"}]},
			    {"pane_id": "bottom", "height": "32px"}]}}`,
		},
		KindElementLayouter: {
			"flow-layouter": `{"descriptor_id": "flow-layouter", "schema_version": "0.5", "code_module_id": "flow-layouter",
			  "config": {"column_gap": "0.75rem", "row_gap": "0.75rem", "horizontal_alignment": "start", "vertical_alignment": "top", "rows_alignment": "top"}}`,
		},
	}
}

// withCopies fills a page's missing class copies from classes, as a page write
// does, and fails the test if a class is missing.
func withCopies(t *testing.T, classes map[Kind]map[string]string, id string, page map[string]any) map[string]any {
	t.Helper()
	issues, err := fillClassCopies(context.Background(), id, page, fixtureLoader(t, classes))
	if err != nil || len(issues) > 0 {
		t.Fatalf("filling class copies: %v %+v", err, issues)
	}
	return page
}

const validApp = `{"descriptor_id": "customer-admin", "schema_version": "0.5", "initial_page": "customer-admin-home"}`

const validFormPage = `{"descriptor_id": "customer-form", "schema_version": "0.5",
 "page_layouter": {"layouter": "single-pane"}, "input": {},
 "save_action": {"action": "save", "table": "kb.wpi_demo_customers", "fields": [{"attribute": "email", "column": "email", "required": true}]},
 "panels": {"main": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [
   {"element": "input", "label": {"en": "Email", "zh-cn": "电子邮箱"}, "data_binding": "$input_doc.email", "binding_mode": "read-only"},
   {"element": "button", "label": "Save", "on_click": {"action": "save-page"}},
   {"element": "button", "label": "Close", "on_click": {"action": "close-page"}}
 ]}}}`

// pageWith is a single-pane page whose panel holds one element instance.
func pageWith(element string) string {
	return `{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"},
	  "panels": {"main": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [` + element + `]}}}`
}

// writePage runs the checks of a page write: class copies are filled in from
// classes, then the page is validated completely.
func writePage(t *testing.T, classes map[Kind]map[string]string, page string) []Issue {
	t.Helper()
	body := decode(t, page)
	id := str(body["descriptor_id"])
	issues, err := fillClassCopies(context.Background(), id, body, fixtureLoader(t, classes))
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) > 0 {
		return issues
	}
	return ValidateDescriptor(KindPage, id, body)
}

func TestValidateApp(t *testing.T) {
	expectIssue(t, ValidateDescriptor(KindApp, "customer-admin", decode(t, validApp)), "")
	cases := []struct{ from, to, want string }{
		{`"initial_page": "customer-admin-home"`, `"initial_page": "Home Page"`, `/initial_page "Home Page" must match`},
		{`, "initial_page": "customer-admin-home"`, ``, `missing attribute "initial_page"`},
		{`"initial_page": "customer-admin-home"`, `"initial_page": "h", "layout_template": "single-pane"`, `unknown attribute "layout_template"`},
		{`"schema_version": "0.5"`, `"schema_version": "0.2"`, `schema_version "0.2" is not supported`},
	}
	for _, c := range cases {
		body := strings.Replace(validApp, c.from, c.to, 1)
		expectIssue(t, ValidateDescriptor(KindApp, "customer-admin", decode(t, body)), c.want)
	}
}

func TestValidateCommon(t *testing.T) {
	body := withCopies(t, testClasses(), "customer-form", decode(t, validFormPage))
	expectIssue(t, ValidateDescriptor(KindPage, "customer-form", body), "")
	expectIssue(t, ValidateDescriptor(KindPage, "other-id", body), `descriptor_id "customer-form" does not match "other-id"`)
	expectIssue(t, ValidateDescriptor(KindPage, "Bad_ID", body), "descriptor ID")

	body["schema_version"] = "0.2"
	expectIssue(t, ValidateDescriptor(KindPage, "customer-form", body), `schema_version "0.2" is not supported by this interpreter for page descriptors (supports "0.5")`)
}

func TestValidatePage(t *testing.T) {
	cases := []struct{ from, to, want string }{
		{`"page_layouter": {"layouter": "single-pane"}`, `"page_layouter": {"layouter": "single-pane"}, "layouter": "flow-layouter"`, `unknown attribute "layouter"`},
		{`"input": {}`, `"input": {"action": "load", "table": "c", "result": "record", "fields": [{"column": "id", "attribute": "id"}], "args": {"id": "$input_doc.id"}}`, "$input_doc is not allowed"},
		{`"input": {}`, `"input": {"action": "save-page"}`, "a save-page action is not allowed here (allowed: load, query, load-file)"},
		{`"input": {}`, `"input": {}, "color": "red"`, `unknown attribute "color"`},
		{`"input": {}`, `"input": {}, "refresh": {"every_seconds": 5}`, `unknown attribute "every_seconds"`},
		{`"save_action": {"action": "save"`, `"save_action": {"action": "load"`, "a load action is not allowed here (allowed: save)"},
		{`"save_action": {"action": "save", "table"`, `"save_action": {"action": "save", "descriptor_id": "x", "table"`, `"descriptor_id" is a class attribute`},
		{`{"action": "save-page"}`, `{"action": "load", "table": "c", "result": "list", "fields": [{"column": "id", "attribute": "id"}]}`, "a load action is not allowed here"},
		{`{"action": "close-page"}`, `{"action": "navigate"}`, `action class "navigate" has no active row to copy`},
		{`{"action": "close-page"}`, `{"page": "x"}`, `missing attribute "action"`},
		{`{"action": "close-page"}`, `{"action": "close-page", "page": "x"}`, `unknown attribute "page"`},
		{`"element": "input"`, `"element": "Input"`, `/panels/main/elem_list/0/element "Input" must match`},
		{`"element": "input"`, `"description": "x", "element": "input"`, `"description" is a class attribute`},
		{`"element": "input"`, `"code_module_id": "button", "element": "input"`, `"code_module_id" is a class attribute`},
		{`{"element": "button", "label": "Save"`, `{"label": "Save"`, `missing attribute "element"`},
		{`"input": {}`, `"input": {}, "page_name": {"en": "Customer", "zh-cn": "客户"}, "description": "One customer",
		  "hori_align": "center", "vert_align": "top", "hori_scrollbar": "hide", "vert_scrollbar": "auto", "width": "100 %", "max_width": "640", "height": "auto", "min_height": "50px",
		  "bkgd_color": "#fff", "frgd_color": "auto", "access_controls": ["admin"],
		  "keywords": ["customer"], "page_tags": ["crm"], "page_policy": "default", "page_style": "plain", "binding_mode": "read-only"`, ""},
		{`"input": {}`, `"input": {"title": "Customer admin", "nested": {"tags": [1, 2]}}`, ""},
		{`"input": {}`, `"input": {}, "page_name": {"zh-cn": "客户"}`, `/page_name texts by language need an "en" entry`},
		{`"input": {}`, `"input": {}, "description": ""`, `/description must be a non-empty string`},
		{`"input": {}`, `"input": {}, "hori_align": "justify"`, `"justify" is not supported`},
		{`"input": {}`, `"input": {}, "hori_scrollbar": "scroll"`, `/hori_scrollbar "scroll" is not supported`},
		{`"input": {}`, `"input": {}, "page_alignment": "center"`, `unknown attribute "page_alignment"`},
		{`"input": {}`, `"input": {}, "left_margin": "30px"`, `unknown attribute "left_margin"`},
		{`"input": {}`, `"input": {}, "status": "draft"`, `unknown attribute "status"`},
		{`"input": {}`, `"input": {}, "vert_align": "expand"`, `"expand" is not supported`},
		{`"input": {}`, `"input": {}, "width": "150%"`, `must be within 0% and 100%`},
		{`"input": {}`, `"input": {}, "min_height": "-5px"`, `/min_height "-5px" must be a non-negative number`},
		{`"input": {}`, `"input": {}, "bkgd_color": "#12"`, `/bkgd_color "#12" must match`},
		{`"input": {}`, `"input": {}, "keywords": [""]`, `/keywords/0 must be a non-empty string`},
		{`"input": {}`, `"input": {}, "access_controls": "admin"`, `/access_controls must be an array`},
		{`"input": {}`, `"input": {}, "page_name": "` + strings.Repeat("n", 101) + `"`, `the limit is 100`},
		{`"input": {}`, `"input": {}, "width": "elastic"`, `/width "elastic" must be a non-negative number`},
		{`"input": {}`, `"input": {}, "max_width": "wide"`, `/max_width "wide" must be a non-negative number`},
		{`"input": {}`, `"input": {}, "binding_mode": "write"`, `/binding_mode "write" is not supported`},
		{`"input": {}`, `"input": "customers"`, "/input must be an object"},
		{`, "input": {}`, `, "input": {}, "elements": []`, `unknown attribute "elements"`},
	}
	for _, c := range cases {
		body := strings.Replace(validFormPage, c.from, c.to, 1)
		if body == validFormPage {
			t.Fatalf("case %q did not change the page", c.want)
		}
		expectIssue(t, writePage(t, testClasses(), body), c.want)
	}
	pilotForm := strings.Replace(validFormPage, `"save_action": {"action": "save"`, `"save_action": "save-customer", "x": {"action": "save"`, 1)
	expectIssue(t, writePage(t, testClasses(), pilotForm), "/save_action must be an object")
}

// TestValidatePanes covers the page layouter copy, pane trees and panels.
func TestValidatePanes(t *testing.T) {
	page := func(layouter string) string {
		return `{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": ` + layouter + `, "panels": {}}`
	}
	tree := func(root string) string { return page(`{"layouter": "single-pane", "root": ` + root + `}`) }
	cases := []struct{ page, want string }{
		{page(`{"layouter": "single-pane"}`), ""},
		{page(`{"layouter": "top-left-middle-right-bottom"}`), ""},
		{tree(`{"pane_id": "main", "width": "auto", "height": "50%", "min_width": "10rem", "max_height": "90%", "resizable": true,
		  "left_margin": "12", "right_margin": "1.5rem", "top_margin": "0px", "bottom_margin": "2em",
		  "hori_align": "left", "vert_align": "center", "hori_scrollbar": "hide", "vert_scrollbar": "auto"}`), ""},
		{tree(`{"pane_id": "page", "split": "columns", "children": [{"pane_id": "a", "width": "elastic"}, {"pane_id": "b", "split": "rows", "children": [{"pane_id": "c"}, {"pane_id": "d"}]}]}`), ""},
		{tree(`{"pane_id": "Main"}`), `/page_layouter/root/pane_id "Main" must match`},
		{tree(`{"pane_id": "page", "split": "rows", "children": [{"pane_id": "a"}, {"pane_id": "a"}]}`), `/page_layouter/root/children/1/pane_id pane ID "a" is used twice`},
		{tree(`{"pane_id": "page", "split": "rows", "children": [{"pane_id": "a"}]}`), `/page_layouter/root/children a split pane needs two or more children`},
		{tree(`{"pane_id": "page", "split": "diagonal", "children": [{"pane_id": "a"}, {"pane_id": "b"}]}`), `"diagonal" is not supported`},
		{tree(`{"pane_id": "page", "split": "rows"}`), `/page_layouter/root/children must be an array`},
		{tree(`{"pane_id": "main", "children": [{"pane_id": "a"}, {"pane_id": "b"}]}`), `/page_layouter/root/children a pane without "split" has no children`},
		{tree(`{"pane_id": "page", "split": "rows", "left_margin": "8px", "children": [{"pane_id": "a"}, {"pane_id": "b"}]}`), ""},
		{tree(`{"pane_id": "main", "width": "wide"}`), `/page_layouter/root/width "wide" must be a non-negative number with an optional unit (px, rem, em, %) or elastic or auto`},
		{tree(`{"pane_id": "main", "min_width": "elastic"}`), `/page_layouter/root/min_width "elastic" must be a non-negative number`},
		{tree(`{"pane_id": "main", "left_margin": "10%"}`), `/page_layouter/root/left_margin "10%" must be a non-negative number with an optional unit (px, rem, em)`},
		{tree(`{"pane_id": "main", "scroll": "none"}`), `/page_layouter/root/scroll unknown attribute "scroll"`},
		{page(`{"layouter": "single-pane", "class": {"descriptor_id": "top-left-middle-right-bottom", "schema_version": "0.5", "code_module_id": "page-layouter", "root": {"pane_id": "main"}}}`),
			`/page_layouter/layouter layouter "single-pane" does not match the descriptor_id "top-left-middle-right-bottom" of its class copy`},
		{page(`{"layouter": "single-pane", "class": {"descriptor_id": "single-pane", "schema_version": "0.5", "code_module_id": "grid", "root": {"pane_id": "main"}}}`),
			`/page_layouter/class/code_module_id class copy: code module "grid" has no page-layouter code in this interpreter`},
		{`{"descriptor_id": "p", "schema_version": "0.5", "panels": {}}`, `missing attribute "page_layouter"`},
		{`{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"},
		  "panels": {"side": {"elem_layouter": {"layouter": "flow-layouter"}}}}`, `/panels/side "side" is not a leaf pane of this page's page layouter`},
		{`{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": {"layouter": "top-left-middle-right-bottom"},
		  "panels": {"middle-row": {"elem_layouter": {"layouter": "flow-layouter"}}}}`, `/panels/middle-row "middle-row" is not a leaf pane`},
		{`{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"},
		  "panels": {"main": {"elem_list": []}}}`, `/panels/main missing attribute "elem_layouter"`},
		{`{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"},
		  "panels": {"main": {"elem_layouter": {"layouter": "flow-layouter", "config": {"column_gap": "1rem", "row_gap": "1rem", "horizontal_alignment": "center", "vertical_alignment": "top", "rows_alignment": "top"}}}}}`, ""},
		{`{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"},
		  "panels": {"main": {"elem_layouter": {"layouter": "flow-layouter", "config": {"column_gap": "1vw"}}}}}`, `/panels/main/elem_layouter/config/column_gap "1vw" must match`},
		{`{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"},
		  "panels": {"main": {"elem_layouter": {"layouter": "flow-layouter", "config": {"gap": "1rem"}}}}}`, `/panels/main/elem_layouter/config/gap unknown attribute "gap"`},
	}
	for _, c := range cases {
		expectIssue(t, writePage(t, testClasses(), c.page), c.want)
	}
	// A page's own root replaces the copy's: its leaves are the page's panes.
	body := withCopies(t, testClasses(), "p", decode(t, `{"descriptor_id": "p", "schema_version": "0.5",
	  "page_layouter": {"layouter": "top-left-middle-right-bottom", "root": {"pane_id": "page", "split": "columns", "children": [{"pane_id": "list"}, {"pane_id": "detail"}]}},
	  "panels": {"list": {"elem_layouter": {"layouter": "flow-layouter"}}, "detail": {"elem_layouter": {"layouter": "flow-layouter"}}}}`))
	expectIssue(t, ValidateDescriptor(KindPage, "p", body), "")
	if leaves, _ := paneLeaves(effectiveRoot(body)); len(leaves) != 2 || !leaves["list"] || !leaves["detail"] {
		t.Fatalf("leaves %v", leaves)
	}
}

// TestValidateLayouterClasses covers page and element layouter class rows.
func TestValidateLayouterClasses(t *testing.T) {
	for _, kind := range []Kind{KindPageLayouter, KindElementLayouter} {
		for id, s := range testClasses()[kind] {
			expectIssue(t, ValidateDescriptor(kind, id, decode(t, s)), "")
		}
	}
	cases := []struct {
		kind     Kind
		id, body string
		want     string
	}{
		{KindPageLayouter, "single-pane", `{"code_module_id": "page-layouter"}`, `missing attribute "root"`},
		{KindPageLayouter, "single-pane", `{"code_module_id": "flow-layouter", "root": {"pane_id": "main"}}`, `code module "flow-layouter" has no page-layouter code`},
		{KindPageLayouter, "single-pane", `{"code_module_id": "page-layouter", "root": {"pane_id": "main", "split": "rows", "children": []}}`, `a split pane needs two or more children`},
		{KindPageLayouter, "single-pane", `{"code_module_id": "page-layouter", "root": {"pane_id": "main"}, "config": {}}`, `unknown attribute "config"`},
		{KindElementLayouter, "flow-layouter", `{"code_module_id": "flow-layouter", "config": {"row_gap": "2"}}`, `/config/row_gap "2" must match`},
		{KindElementLayouter, "flow-layouter", `{"code_module_id": "page-layouter", "config": {}}`, `code module "page-layouter" has no element-layouter code`},
		{KindElementLayouter, "flow-layouter", `{"code_module_id": "flow-layouter", "config": {}, "schema_version": "0.4"}`, `schema_version "0.4" is not supported by this interpreter for element-layouter descriptors (supports "0.5")`},
	}
	for _, c := range cases {
		body := decode(t, c.body)
		body["descriptor_id"] = c.id
		if _, ok := body["schema_version"]; !ok {
			body["schema_version"] = schemaVersionOf(c.kind)
		}
		expectIssue(t, ValidateDescriptor(c.kind, c.id, body), c.want)
	}
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
		{`{"element": "list", "source": {"action": "query", "sql_stmt_id": "x", "result": "list"}, "columns": [{"field": "a", "label": "A"}], "presentation": "cards"}`, `"cards" is not supported`},
		{`{"element": "list", "source": {"action": "save", "table": "c", "fields": [{"attribute": "a", "column": "a"}]}}`, "a save action is not allowed here (allowed: load, query)"},
		{`{"element": "list", "source": {"action": "query", "sql_stmt_id": "wpi-demo-customer-directory", "result": "list"}, "columns": [{"field": "email", "label": "Email"}]}`, ""},
		{`{"element": "list", "source": {"action": "load-file", "file_id": "f"}}`, "a load-file action is not allowed here (allowed: load, query)"},
		{`{"element": "list", "source": {"title": "static"}}`, `/panels/main/elem_list/0/source missing attribute "action"`},
		{`{"element": "menu", "initial_item": "a", "items": [{"item_id": "a", "label": "A", "on_click": {"action": "open-page", "page": "p", "pane": "middle"}}, {"item_id": "f", "label": "F", "children": [{"item_id": "b", "label": "B", "on_click": {"action": "open-page", "page": "q", "pane": "middle"}}]}]}`, ""},
		{`{"element": "menu", "items": [{"item_id": "a", "label": "A", "on_click": {"action": "load"}}]}`, "a load action is not allowed here"},
		{`{"element": "menu", "items": [{"item_id": "a", "label": "A"}]}`, "exactly one of"},
		{`{"element": "menu", "items": [{"item_id": "a", "label": "A", "on_click": {"action": "close-page"}}, {"item_id": "a", "label": "B", "on_click": {"action": "close-page"}}]}`, `menu item ID "a" is used twice`},
		{`{"element": "slider", "label": "x"}`, `/panels/main/elem_list/0/element element class "slider" has no active row to copy`},
		{`{"element": "input", "label": "x", "data_binding": "$input_doc.x", "on_click": {"action": "close-page"}}`, `/panels/main/elem_list/0/on_click element class "input" has no attribute "on_click"`},
	}
	for _, c := range cases {
		expectIssue(t, writePage(t, testClasses(), pageWith(c.body)), c.want)
	}
}

// TestValidateActionInstances covers action instances in each kind of place.
func TestValidateActionInstances(t *testing.T) {
	onInput := func(a string) string {
		return `{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"}, "input": ` + a + `}`
	}
	onSave := func(a string) string {
		return `{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"}, "save_action": ` + a + `}`
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
		{onClick(`{"action": "open-page", "page": "customer-form", "pane": "right", "input": {"action": "save-page"}}`), "a save-page action is not allowed here (allowed: load, query, load-file)"},
		{onClick(`{"action": "open-page", "page": "customer-form", "pane": "right", "input": {}}`), ""},
		{onClick(`{"action": "save-page"}`), ""},
		{onInput(`{"action": "query", "sql_stmt_id": "wpi-demo-customer-by-id", "result": "record", "params": ["$args.id", 5, "x", null]}`), ""},
		{onInput(`{"action": "query", "sql_stmt_id": "Customers By ID", "result": "record"}`), "/input/sql_stmt_id"},
		{onInput(`{"action": "query", "sql_stmt_id": "x", "result": "table"}`), `"table" is not supported`},
		{onInput(`{"action": "query", "sql_stmt_id": "x", "result": "list", "params": ["$input_doc.id"]}`), "/input/params/0 $input_doc is not allowed"},
		{onInput(`{"action": "query", "sql_stmt_id": "x", "result": "list", "params": {"id": 1}}`), "/input/params must be an array"},
		{onInput(`{"action": "query", "sql_stmt_id": "x", "result": "list", "sql": "SELECT 1"}`), `unknown attribute "sql"`},
		{onInput(`{"action": "load-file", "file_id": "wpi-demo-about"}`), ""},
		{onInput(`{"action": "load-file", "file_id": "../etc/passwd"}`), "/input/file_id"},
		{onClick(`{"action": "open-page", "page": "about", "pane": "right", "input": {"action": "load-file", "file_id": "wpi-demo-about"}}`), ""},
		{onClick(`{"action": "open-page", "page": "about", "pane": "right", "input": {"title": "Static"}}`), ""},
		{onClick(`{"action": "open-page", "page": "f", "pane": "right", "args": {"id": "$input_doc.id"}, "input": {"action": "query", "sql_stmt_id": "x", "result": "record", "params": ["$args.id"]}}`), ""},
		{onClick(`{"page": "about"}`), `missing attribute "action"`},
		{onClick(`{"action": "save", "table": "c", "fields": [{"attribute": "a", "column": "a"}]}`), ""},
	}
	for _, c := range cases {
		expectIssue(t, writePage(t, testClasses(), c.page), c.want)
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
		{KindElement, "slider", `{"code_module_id": "slider"}`, `/code_module_id code module "slider" has no code in this interpreter`},
		{KindElement, "slider", `{"code_module_id": "Slider"}`, `/code_module_id must be a code module ID`},
		{KindAction, "navigate", `{}`, `action class "navigate" has no code in this interpreter`},
		{KindAction, "save-page", `{"code_module_id": "save-page"}`, `/code_module_id an action class's ID names its code`},
		// Attributes the module does not define are warnings (classWarnings), not errors.
		{KindElement, "input", `{"label": {"default_value": null}, "data_binding": {"default_value": null}, "binding_mode": {"default_value": "read-write"}, "on_row_click": {}, "glow": {"default_value": 1}}`, ""},
		{KindElement, "fancy-button", `{"code_module_id": "button", "label": {"default_value": "Go", "instance_modifiable": false}, "on_click": {"default_value": null}}`, ""},
		{KindElement, "button", `{"label": {"instance_modifiable": false}, "on_click": {"default_value": null}}`, `/label a fixed attribute needs a default_value other than null`},
		{KindElement, "button", `{"label": {"default_value": null, "instance_modifiable": false}, "on_click": {"default_value": null}}`, `/label a fixed attribute needs a default_value other than null`},
		{KindElement, "button", `{"label": {"default_value": null}, "on_click": {"default_value": null, "instance_modifiable": false}}`, `/on_click attribute "on_click" holds actions, so it cannot be fixed`},
		{KindElement, "button", `{"label": {"default_value": "Go", "instance_modifiable": "no"}, "on_click": {"default_value": null}}`, `/label/instance_modifiable must be true or false`},
		{KindElement, "input", `{"label": {"default_value": null}, "binding_mode": {"default_value": "read-write"}}`, `must give "data_binding" a default_value or null`},
		{KindElement, "input", `{"label": {"default_value": null}, "data_binding": {"help_tip": "Bound attribute"}, "binding_mode": {"default_value": "read-write"}}`, `must give "data_binding" a default_value or null`},
		{KindElement, "input", `{"label": {"default_value": null}, "data_binding": {"default_value": null}, "binding_mode": {"default_value": "write"}}`, `"write" is not supported`},
		{KindElement, "input", `{"label": {"default_value": null}, "data_binding": {"default_value": null}, "binding_mode": {"default_value": "read-write"}, "description": {"zh-cn": "输入框"}}`, `need an "en" entry`},
		{KindElement, "input", `{"label": null, "data_binding": {"default_value": null}, "binding_mode": {"default_value": "read-write"}}`, `/label must be an object with an optional "help_tip"`},
		{KindElement, "input", `{"label": {"default_value": null, "hint": "x"}, "data_binding": {"default_value": null}, "binding_mode": {"default_value": "read-write"}}`, `/label/hint unknown attribute "hint"`},
		{KindElement, "input", `{"label": {"help_tip": {"zh-cn": "标题"}, "default_value": null}, "data_binding": {"default_value": null}, "binding_mode": {"default_value": "read-write"}}`, `/label/help_tip texts by language need an "en" entry`},
		{KindElement, "input", `{"label": {"default_value": null}, "data_binding": {"default_value": null}, "binding_mode": {"default_value": "read-write"}, "implement_notes": ""}`, `/implement_notes must be a non-empty string`},
		{KindElement, "button", `{"label": {"default_value": "Go"}, "on_click": {"default_value": {"action": "close-page"}}}`, `/on_click/default_value attribute "on_click" holds actions`},
		{KindElement, "list", `{"source": {"default_value": null}, "columns": {"default_value": null}, "presentation": {"default_value": "rows"}, "on_row_click": {"default_value": {"action": "close-page"}}}`, `attribute "on_row_click" holds actions`},
		{KindElement, "input", `{"label": {"default_value": null}, "data_binding": {"default_value": null}, "binding_mode": {"default_value": "read-write"}, "schema_version": "0.3"}`, `schema_version "0.3" is not supported by this interpreter for element descriptors (supports "0.4")`},
		{KindAction, "open-page", `{"page": null, "pane": null, "input": {}}`, `attribute "input" holds actions`},
		{KindAction, "save", `{"table": null, "mode": "merge", "key": "id", "fields": null}`, `"merge" is not supported`},
		{KindAction, "save-page", `{"args": {}}`, `unknown attribute "args"`},
	}
	for _, c := range cases {
		body := decode(t, c.body)
		body["descriptor_id"] = c.id
		if _, ok := body["schema_version"]; !ok {
			body["schema_version"] = schemaVersionOf(c.kind)
		}
		if _, ok := body["code_module_id"]; !ok && c.kind == KindElement {
			body["code_module_id"] = c.id
		}
		expectIssue(t, ValidateDescriptor(c.kind, c.id, body), c.want)
	}
	// An active element class must name its code module.
	expectIssue(t, ValidateDescriptor(KindElement, "button", decode(t, `{"descriptor_id": "button", "schema_version": "0.4", "label": {"default_value": null}, "on_click": {"default_value": null}}`)),
		`/code_module_id missing attribute "code_module_id"`)
}

func TestClassWarnings(t *testing.T) {
	row := decode(t, `{"descriptor_id": "fancy-button", "schema_version": "0.4", "code_module_id": "button", "description": "x",
		"label": {"default_value": null}, "on_click": {"default_value": null}, "width": {"default_value": "120px"},
		"glow": {"default_value": 1}, "label_hori_align": {"default_value": "center"}}`)
	var got []string
	for _, w := range classWarnings("fancy-button", row) {
		got = append(got, w.Path+" "+w.Message)
	}
	want := `/glow code module "button" does not support attribute "glow", so it has no effect|` +
		`/label_hori_align code module "button" does not support attribute "label_hori_align", so it has no effect`
	if strings.Join(got, "|") != want {
		t.Fatalf("warnings %q", got)
	}
	// A draft whose module is missing or has no code yet has none.
	delete(row, "code_module_id")
	expectIssue(t, classWarnings("fancy-button", row), "")
	row["code_module_id"] = "slider"
	expectIssue(t, classWarnings("fancy-button", row), "")
}

func TestValidateDraftClass(t *testing.T) {
	valid := `{"descriptor_id": "slider", "schema_version": "0.4", "description": {"en": "A slider", "zh-cn": "滑块"},
		"implement_notes": "Render a range input bound to data_binding.",
		"label": {"help_tip": {"en": "Caption", "zh-cn": "标题"}, "default_value": null}, "min": {"default_value": 0}, "step": {"default_value": 5},
		"on_change": {"help_tip": "Runs when the value changes"}, "width": {"default_value": "240px"}, "binding_mode": {"default_value": "read-write"}}`
	expectIssue(t, ValidateDraftClass(decode(t, valid)), "")
	// A class that has code may be drafted too; its code is checked when it is made active.
	expectIssue(t, ValidateDraftClass(decode(t, `{"descriptor_id": "input", "schema_version": "0.4", "description": "x"}`)), "")
	// A draft may name a code module whose code is not written yet.
	expectIssue(t, ValidateDraftClass(decode(t, `{"descriptor_id": "dial", "schema_version": "0.4", "description": "x", "code_module_id": "slider"}`)), "")
	cases := []struct{ body, want string }{
		{`{"descriptor_id": "slider", "description": "x", "code_module_id": "Slider_1"}`, `/code_module_id must be a code module ID`},
		{`{"descriptor_id": "slider", "description": "x", "style": {"instance_modifiable": false}}`, `/style a fixed attribute needs a default_value`},
		{`{"descriptor_id": "slider", "description": "x", "style": {"default_value": "fancy", "instance_modifiable": false}}`, ""},
		{`{"descriptor_id": "Slider", "description": "x"}`, `/descriptor_id descriptor ID "Slider"`},
		{`{"descriptor_id": "slider", "schema_version": "0.2", "description": "x"}`, `schema_version "0.2" is not supported`},
		{`{"descriptor_id": "slider"}`, `/description a draft class needs a description`},
		{`{"descriptor_id": "slider", "description": {"zh-cn": "滑块"}}`, `need an "en" entry`},
		{`{"descriptor_id": "slider", "description": "x", "implement_notes": ""}`, `/implement_notes must be a non-empty string`},
		{`{"descriptor_id": "slider", "description": "x", "element": {}}`, `"element" cannot be an attribute name`},
		{`{"descriptor_id": "slider", "description": "x", "class": {}}`, `"class" cannot be an attribute name`},
		{`{"descriptor_id": "slider", "description": "x", "max-value": {"default_value": 1}}`, `"max-value" cannot be an attribute name`},
		{`{"descriptor_id": "slider", "description": "x", "max": 1}`, `/max must be an object with an optional "help_tip"`},
		{`{"descriptor_id": "slider", "description": "x", "max": {"value": 1}}`, `/max/value unknown attribute "value"`},
		{`{"descriptor_id": "slider", "description": "x", "max": {"help_tip": ""}}`, `/max/help_tip must be a non-empty string`},
		{`{"descriptor_id": "slider", "description": "x", "on_click": {"default_value": {"action": "close-page"}}}`, `/on_click/default_value attribute "on_click" holds actions`},
		{`{"descriptor_id": "slider", "description": "x", "width": {"default_value": "wide"}}`, `/width "wide" must match`},
		{`{"descriptor_id": "slider", "description": "x", "label": {"default_value": ""}}`, `/label must be a non-empty string`},
		{`{"descriptor_id": "slider", "description": "x", "data_binding": {"default_value": "$record.x"}}`, `must be a $input_doc path`},
		{`{"descriptor_id": "slider", "description": "x", "binding_mode": {"default_value": "write"}}`, `"write" is not supported`},
	}
	for _, c := range cases {
		body := decode(t, c.body)
		if _, ok := body["schema_version"]; !ok {
			body["schema_version"] = ElementSchemaVersion
		}
		expectIssue(t, ValidateDraftClass(body), c.want)
	}
}

// TestInstancesAgainstCopies covers the checks made against the class copies
// instances hold, and the merge with them.
func TestInstancesAgainstCopies(t *testing.T) {
	check := func(t *testing.T, classes map[Kind]map[string]string, page string) ([]mergedElement, []mergedAction, []Issue) {
		t.Helper()
		body := decode(t, page)
		id := str(body["descriptor_id"])
		if issues, err := fillClassCopies(context.Background(), id, body, fixtureLoader(t, classes)); err != nil || len(issues) > 0 {
			return nil, nil, issues
		}
		issues, els, acts := validatePage(id, body)
		return els, acts, issues
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
		{"null attribute not set", `{"element": "input", "label": "x"}`, `attribute "data_binding" has no default in this instance's copy of element class "input", so the instance must set it`, nil},
		{"text needs text or binding", `{"element": "text"}`, "exactly one of", nil},
		{"text with both", `{"element": "text", "text": "x", "data_binding": "$input_doc.x"}`, "exactly one of", nil},
		{"initial item is a folder", `{"element": "menu", "initial_item": "f", "items": [{"item_id": "f", "label": "F", "children": [{"item_id": "b", "label": "B", "on_click": {"action": "close-page"}}]}]}`,
			`initial_item "f" is not a menu item that opens a page`, nil},
		{"class row missing", `{"element": "text", "text": "x"}`, `element class "text" has no active row to copy`,
			func(c map[Kind]map[string]string) { delete(c[KindElement], "text") }},
		{"code-required attribute missing after merge", `{"element": "list", "source": {"action": "load", "table": "c", "result": "list", "fields": [{"column": "a", "attribute": "a"}]}}`,
			`/panels/main/elem_list/0/class class copy: the class must give "columns" a default_value or null`, func(c map[Kind]map[string]string) {
				c[KindElement]["list"] = `{"descriptor_id": "list", "schema_version": "0.4", "code_module_id": "list", "source": {"default_value": null}, "presentation": {"default_value": "rows"}}`
			}},
		{"attribute the class does not name", `{"element": "button", "label": "Go", "on_click": {"action": "close-page"}, "input_type": "email"}`,
			`/panels/main/elem_list/0/input_type element class "button" has no attribute "input_type"`, nil},
		{"fixed attribute set", `{"element": "fancy-button", "label": "Go", "on_click": {"action": "close-page"}}`,
			`/panels/main/elem_list/0/label attribute "label" is fixed by element class "fancy-button"; an instance cannot set it`, addFancyButton},
		{"shared module checks its required attributes", `{"element": "fancy-button"}`,
			`attribute "on_click" has no default in this instance's copy of element class "fancy-button"`, addFancyButton},
		{"shared module checks values", `{"element": "big-list", "source": {"action": "query", "sql_stmt_id": "x", "result": "list"}, "columns": [{"field": "a", "label": "A"}], "presentation": "cards"}`,
			`/panels/main/elem_list/0/presentation "cards" is not supported`, func(c map[Kind]map[string]string) {
				c[KindElement]["big-list"] = `{"descriptor_id": "big-list", "schema_version": "0.4", "code_module_id": "list",
					"source": {"default_value": null}, "presentation": {"default_value": "rows"}, "columns": {"default_value": null}}`
			}},
		{"unsupported attribute set", `{"element": "fancy-button", "on_click": {"action": "close-page"}, "glow": [1, 2]}`, "", addFancyButton},
		{"action null attribute not set", `{"element": "button", "label": "Go", "on_click": {"action": "open-page", "page": "x"}}`,
			`attribute "pane" has no default in this instance's copy of action class "open-page"`, nil},
		{"action class row missing", `{"element": "button", "label": "Go", "on_click": {"action": "save-page"}}`,
			`action class "save-page" has no active row to copy`, func(c map[Kind]map[string]string) { delete(c[KindAction], "save-page") }},
		{"query without result", `{"element": "list", "source": {"action": "query", "sql_stmt_id": "x"}, "columns": [{"field": "a", "label": "A"}]}`,
			`attribute "result" has no default in this instance's copy of action class "query"`, nil},
		{"broken class copied", `{"element": "input", "label": "x", "data_binding": "$input_doc.x"}`,
			`/panels/main/elem_list/0/class/binding_mode class copy: "write" is not supported`, func(c map[Kind]map[string]string) {
				c[KindElement]["input"] = strings.Replace(c[KindElement]["input"], `"binding_mode": {"default_value": "read-write"}`, `"binding_mode": {"default_value": "write"}`, 1)
			}},
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
	t.Run("fixed value merged", func(t *testing.T) {
		classes := testClasses()
		addFancyButton(classes)
		els, _, issues := check(t, classes, pageWith(`{"element": "fancy-button", "on_click": {"action": "close-page"}}`))
		expectIssue(t, issues, "")
		if len(els) != 1 || str(els[0].body["label"]) != "Go!" || els[0].body["glow"] != nil {
			t.Fatalf("elements %+v", els)
		}
	})
	t.Run("copy, not the stored class, is used", func(t *testing.T) {
		classes := testClasses()
		body := withCopies(t, classes, "p", decode(t, pageWith(`{"element": "input", "label": "x", "data_binding": "$input_doc.x"}`)))
		// The stored class changes after the page was written; the page keeps its copy.
		classes[KindElement]["input"] = strings.Replace(classes[KindElement]["input"], `"width": {"default_value": "elastic"}`, `"width": {"default_value": "240px"}`, 1)
		issues, err := fillClassCopies(context.Background(), "p", body, fixtureLoader(t, classes))
		if err != nil || len(issues) > 0 {
			t.Fatal(err, issues)
		}
		_, els, _ := validatePage("p", body)
		if len(els) != 1 || str(els[0].body["width"]) != "elastic" {
			t.Fatalf("elements %+v", els)
		}
	})
	t.Run("name differs from copy", func(t *testing.T) {
		body := withCopies(t, testClasses(), "p", decode(t, pageWith(`{"element": "input", "label": "x", "data_binding": "$input_doc.x"}`)))
		panel := body["panels"].(map[string]any)["main"].(map[string]any)
		panel["elem_list"].([]any)[0].(map[string]any)["element"] = "button"
		expectIssue(t, ValidateDescriptor(KindPage, "p", body), `/panels/main/elem_list/0/element element "button" does not match the descriptor_id "input" of its class copy`)
	})
}

// addFancyButton adds a class drawn by the button code module that fixes its
// label and names an attribute the module does not support.
func addFancyButton(c map[Kind]map[string]string) {
	c[KindElement]["fancy-button"] = `{"descriptor_id": "fancy-button", "schema_version": "0.4", "code_module_id": "button",
		"label": {"default_value": "Go!", "instance_modifiable": false}, "on_click": {"default_value": null}, "glow": {}}`
}

func TestPageWideBindingMode(t *testing.T) {
	page := `{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"}, "binding_mode": "read-only",
	  "panels": {"main": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [
	  {"element": "input", "label": "A", "data_binding": "$input_doc.a"},
	  {"element": "input", "label": "B", "data_binding": "$input_doc.b", "binding_mode": "read-write"},
	  {"element": "select", "label": "C", "data_binding": "$input_doc.c", "options": [{"value": 1, "label": "One"}]},
	  {"element": "text", "text": "x"},
	  {"element": "editable-input", "label": "D", "data_binding": "$input_doc.d"}]}}}`
	classes := testClasses()
	// A class that leaves binding_mode to every instance is satisfied by the page.
	classes[KindElement]["select"] = strings.Replace(classes[KindElement]["select"], `"binding_mode": {"default_value": "read-write"}`, `"binding_mode": {"default_value": null}`, 1)
	// A class that fixes binding_mode keeps it: the page's does not fill it in.
	classes[KindElement]["editable-input"] = `{"descriptor_id": "editable-input", "schema_version": "0.4", "code_module_id": "input",
		"label": {"default_value": null}, "data_binding": {"default_value": null}, "binding_mode": {"default_value": "read-write", "instance_modifiable": false}}`
	issues, els, _ := validatePage("p", withCopies(t, classes, "p", decode(t, page)))
	expectIssue(t, issues, "")
	var got []string
	for _, e := range els {
		got = append(got, str(e.body["binding_mode"]))
	}
	if strings.Join(got, ",") != "read-only,read-write,read-only,,read-write" {
		t.Fatalf("binding modes %q; want instance, then page, then class, none for text, and the fixed one", got)
	}
}

func TestStaticInputPositions(t *testing.T) {
	page := decode(t, `{"input": {"title": "x"}, "panels": {"main": {"elem_list": [
	  {"element": "button", "on_click": {"page": "p"}},
	  {"element": "button", "on_click": {"action": "open-page", "input": {"a": 1}}}]}}}`)
	got := map[string]bool{}
	for _, p := range actionPositions(page) {
		got[p.path] = p.isStatic()
	}
	want := map[string]bool{"/input": true, "/panels/main/elem_list/0/on_click": false, "/panels/main/elem_list/1/on_click": false, "/panels/main/elem_list/1/on_click/input": true}
	if len(got) != len(want) {
		t.Fatalf("positions %v", got)
	}
	for path, static := range want {
		if got[path] != static {
			t.Fatalf("%s static = %v, want %v", path, got[path], static)
		}
	}
}

func TestActionPositions(t *testing.T) {
	page := decode(t, `{"input": {}, "save_action": {"action": "save"}, "panels": {
	  "main": {"elem_list": [
	    {"element": "list", "source": {"action": "load"}, "on_row_click": {"action": "open-page", "input": {"action": "load"}}},
	    {"element": "menu", "items": [{"item_id": "f", "children": [{"item_id": "a", "on_click": {"action": "close-page"}}]}]}]},
	  "aside": {"elem_list": [{"element": "input", "on_click": {"action": "close-page"}}]}}}`)
	var got []string
	for _, p := range actionPositions(page) {
		got = append(got, p.path)
	}
	// Panels in pane ID order. The attribute names are reserved for actions,
	// whatever the element's class: the input's on_click is a position too (its
	// class copy decides whether the instance may set it).
	want := "/input /save_action /panels/aside/elem_list/0/on_click /panels/main/elem_list/0/source /panels/main/elem_list/0/on_row_click /panels/main/elem_list/1/items/0/children/0/on_click /panels/main/elem_list/0/on_row_click/input"
	if strings.Join(got, " ") != want {
		t.Fatalf("got  %s\nwant %s", strings.Join(got, " "), want)
	}
}

func TestFillClassCopies(t *testing.T) {
	body := decode(t, `{"descriptor_id": "p", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"},
	  "input": {"title": "static"},
	  "panels": {"main": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [
	    {"element": "input", "label": "A", "data_binding": "$input_doc.a"},
	    {"element": "input", "label": "B", "data_binding": "$input_doc.b", "class": {"descriptor_id": "input", "old": true}},
	    {"element": "button", "label": "Go", "on_click": {"action": "open-page", "page": "q", "pane": "main", "input": {"action": "load-file", "file_id": "f"}}}]}}}`)
	issues, err := fillClassCopies(context.Background(), "p", body, fixtureLoader(t, testClasses()))
	if err != nil || len(issues) > 0 {
		t.Fatal(err, issues)
	}
	panel := body["panels"].(map[string]any)["main"].(map[string]any)
	list := panel["elem_list"].([]any)
	copyOf := func(m any) map[string]any { c, _ := m.(map[string]any)[copyKey].(map[string]any); return c }
	if str(copyOf(body["page_layouter"])["code_module_id"]) != "page-layouter" || str(copyOf(panel["elem_layouter"])["code_module_id"]) != "flow-layouter" {
		t.Fatal("layouter copies missing")
	}
	if str(copyOf(list[0])["descriptor_id"]) != "input" || copyOf(list[1])["old"] != true {
		t.Fatalf("element copies %v / %v", copyOf(list[0]), copyOf(list[1]))
	}
	click := list[2].(map[string]any)["on_click"].(map[string]any)
	if str(copyOf(click)["descriptor_id"]) != "open-page" || str(copyOf(click["input"])["descriptor_id"]) != "load-file" {
		t.Fatal("action copies missing")
	}
	if _, has := body["input"].(map[string]any)[copyKey]; has {
		t.Fatal("static input must not get a copy")
	}
	// Two instances of one class get copies of their own.
	body2 := withCopies(t, testClasses(), "p", decode(t, pageWith(`{"element": "text", "text": "a"}, {"element": "text", "text": "b"}`)))
	l2 := body2["panels"].(map[string]any)["main"].(map[string]any)["elem_list"].([]any)
	copyOf(l2[0])["x"] = 1
	if copyOf(l2[1])["x"] != nil {
		t.Fatal("copies must not share a map")
	}
}
