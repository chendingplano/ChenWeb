package wpi

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
)

// fixture is a small app: a home page with a menu in pane nav, a list page
// opened in pane left and a form page opened in pane right; the list and the
// form open each other, so the page walk has to stop at a cycle. Pages are
// written without class copies; fixtureLoader fills them in, as a page write
// would have.
func fixture() map[Kind]map[string]string {
	fx := testClasses()
	fx[KindApp] = map[string]string{
		"demo": `{"descriptor_id": "demo", "schema_version": "0.5", "initial_page": "home"}`,
	}
	fx[KindPage] = map[string]string{
		"home": `{"descriptor_id": "home", "schema_version": "0.5",
		  "page_layouter": {"layouter": "single-pane", "root": {"pane_id": "page", "split": "columns", "children": [
		    {"pane_id": "nav", "width": "200px"}, {"pane_id": "left"}, {"pane_id": "right"}]}},
		  "panels": {"nav": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [{"element": "menu", "initial_item": "people",
		    "items": [{"item_id": "people", "label": "People", "on_click": {"action": "open-page", "page": "list-page", "pane": "left"}}]}]}}}`,
		"list-page": `{"descriptor_id": "list-page", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"},
		  "panels": {"main": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [{"element": "list",
		    "source": {"action": "load", "table": "people", "result": "list", "fields": [{"column": "id", "attribute": "id"}, {"column": "name", "attribute": "name"}]},
		    "columns": [{"field": "name", "label": "Name"}],
		    "on_row_click": {"action": "open-page", "page": "form-page", "pane": "right", "args": {"id": "$record.id"},
		      "input": {"action": "load", "table": "people", "result": "record", "where": {"id": "$args.id"},
		        "fields": [{"column": "id", "attribute": "id"}, {"column": "name", "attribute": "name"}]}}}]}}}`,
		"form-page": `{"descriptor_id": "form-page", "schema_version": "0.5", "page_layouter": {"layouter": "single-pane"},
		  "input": {}, "save_action": {"action": "save", "table": "people", "fields": [{"attribute": "name", "column": "name", "required": true}]},
		  "panels": {"main": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [{"element": "input", "label": "Name", "data_binding": "$input_doc.name"},
		    {"element": "button", "label": "Save", "on_click": {"action": "save-page"}},
		    {"element": "button", "label": "Back", "on_click": {"action": "open-page", "page": "list-page", "pane": "left"}}]}}}`,
	}
	return fx
}

// fixtureLoader serves fx's rows. Pages get their missing class copies from
// fx's classes when they are loaded, so a test can still edit a class row.
func fixtureLoader(t *testing.T, fx map[Kind]map[string]string) Loader {
	var loader Loader
	loader = func(ctx context.Context, kind Kind, ids []string) (map[string]Descriptor, error) {
		out := map[string]Descriptor{}
		for id, s := range fx[kind] {
			if ids != nil && !slices.Contains(ids, id) {
				continue
			}
			body, raw := decode(t, s), []byte(s)
			if kind == KindPage {
				issues, err := fillClassCopies(ctx, id, body, loader)
				if err != nil || len(issues) > 0 {
					t.Fatalf("filling class copies of page %s: %v %+v", id, err, issues)
				}
				if raw, err = json.Marshal(body); err != nil {
					t.Fatal(err)
				}
			}
			out[id] = Descriptor{Kind: kind, ID: id, Version: 1, Raw: raw, Body: body}
		}
		return out, nil
	}
	return loader
}

func build(t *testing.T, fx map[Kind]map[string]string, tables map[string][]string) (*BundleResult, error) {
	t.Helper()
	columns := func(_ context.Context, table string) (map[string]bool, error) {
		cols, ok := tables[table]
		if !ok {
			return nil, nil
		}
		m := map[string]bool{}
		for _, c := range cols {
			m[c] = true
		}
		return m, nil
	}
	return BuildBundle(context.Background(), "demo", fixtureLoader(t, fx), columns, registeredIn(nil))
}

// registeredIn fakes the stored statements and files, by kind, and the active
// modules of kb.code_modules: every module with code, unless rows names them.
func registeredIn(rows map[string][]string) Registered {
	withCode := map[string][]string{
		storedModule:                slices.Collect(maps.Keys(codeModules)),
		storedPageLayouterModule:    pageLayouterModules,
		storedElementLayouterModule: slices.Collect(maps.Keys(elementLayouterModules)),
	}
	return func(_ context.Context, kind string, names []string) (map[string]bool, error) {
		known, given := rows[kind]
		if !given {
			known = withCode[kind]
		}
		out := map[string]bool{}
		for _, n := range names {
			if slices.Contains(known, n) {
				out[n] = true
			}
		}
		return out, nil
	}
}

// A class copy whose code module is suspended, or has no row in
// kb.code_modules, stops the apps that use it (OpenSpec wpi-code-modules, design
// D9; wpi-pages-own-panes, design D6).
func TestBuildBundleNeedsActiveCodeModules(t *testing.T) {
	columns := func(_ context.Context, table string) (map[string]bool, error) {
		return map[string]bool{"id": true, "name": true}, nil
	}
	cases := []struct {
		rows map[string][]string
		want string
	}{
		{map[string][]string{storedModule: {"input", "list", "menu"}},
			`/panels/main/elem_list/1/class/code_module_id code module "button" is not an active element module in kb.code_modules`},
		{map[string][]string{storedPageLayouterModule: {}},
			`/page_layouter/class/code_module_id code module "page-layouter" is not an active page-layouter module`},
		{map[string][]string{storedElementLayouterModule: {}},
			`/panels/main/elem_layouter/class/code_module_id code module "flow-layouter" is not an active element-layouter module`},
	}
	for _, c := range cases {
		res, err := BuildBundle(context.Background(), "demo", fixtureLoader(t, fixture()), columns, registeredIn(c.rows))
		if err != nil {
			t.Fatal(err)
		}
		if res.Bundle != nil {
			t.Fatal("a bundle with a suspended module must not be built")
		}
		expectIssue(t, res.Errors, c.want)
	}
}

var peopleTable = map[string][]string{"people": {"id", "name"}}

func TestBuildBundleValid(t *testing.T) {
	res, err := build(t, fixture(), peopleTable)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || len(res.Warnings) != 0 {
		t.Fatalf("errors %+v warnings %+v", res.Errors, res.Warnings)
	}
	b := res.Bundle
	if len(b.Pages) != 3 {
		t.Fatalf("pages %d", len(b.Pages))
	}
	// Pages are sent with their class copies.
	form := decode(t, string(b.Pages["form-page"]))
	first := form["panels"].(map[string]any)["main"].(map[string]any)["elem_list"].([]any)[0].(map[string]any)
	if str(first[copyKey].(map[string]any)["descriptor_id"]) != "input" {
		t.Fatalf("form-page without class copies: %s", b.Pages["form-page"])
	}
}

func TestBuildBundleNotFound(t *testing.T) {
	fx := fixture()
	delete(fx[KindApp], "demo")
	if _, err := build(t, fx, peopleTable); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildBundleIgnoresBrokenUnusedClass(t *testing.T) {
	fx := fixture()
	fx[KindElement]["select"] = `{"descriptor_id": "select", "schema_version": "0.3"}`
	res, err := build(t, fx, peopleTable)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || res.Bundle == nil {
		t.Fatalf("errors %+v", res.Errors)
	}
}

func TestBuildBundleErrors(t *testing.T) {
	replace := func(kind Kind, id, from, to string) func(fx map[Kind]map[string]string) {
		return func(fx map[Kind]map[string]string) {
			if !strings.Contains(fx[kind][id], from) {
				panic("fixture does not contain " + from)
			}
			fx[kind][id] = strings.Replace(fx[kind][id], from, to, 1)
		}
	}
	cases := []struct {
		name   string
		edit   func(fx map[Kind]map[string]string)
		tables map[string][]string
		want   string
	}{
		{"missing initial page", replace(KindApp, "demo", `"initial_page": "home"`, `"initial_page": "start"`), peopleTable,
			`/initial_page page "start" does not exist`},
		{"missing page", replace(KindPage, "list-page", `"page": "form-page"`, `"page": "nowhere"`), peopleTable,
			`/panels/main/elem_list/0/on_row_click/page page "nowhere" does not exist`},
		{"unknown pane", replace(KindPage, "list-page", `"pane": "right"`, `"pane": "side"`), peopleTable,
			`/panels/main/elem_list/0/on_row_click/pane pane "side" is not a leaf pane of page "list-page" or of a page it is opened in (list-page in home)`},
		{"open-page in a button to an unknown pane", replace(KindPage, "form-page", `"pane": "left"`, `"pane": "nowhere"`), peopleTable,
			`/panels/main/elem_list/2/on_click/pane pane "nowhere" is not a leaf pane of page "form-page"`},
		{"split pane is not a target", replace(KindPage, "list-page", `"pane": "right"`, `"pane": "page"`), peopleTable,
			`pane "page" is not a leaf pane of page "list-page"`},
		{"missing column", nil, map[string][]string{"people": {"id"}}, `column "name" does not exist in table "people"`},
		{"missing table", nil, map[string][]string{}, `table "people" does not exist`},
		{"wrong class in a position", replace(KindPage, "list-page", `"source": {"action": "load"`, `"source": {"action": "save"`), peopleTable,
			"a save action is not allowed here (allowed: load, query)"},
		{"null attribute not set", replace(KindPage, "form-page", `"label": "Name", `, ``), peopleTable,
			`attribute "label" has no default in this instance's copy of element class "input"`},
		{"structure error in a reached page", replace(KindPage, "form-page", `"$input_doc.name"`, `"$input_doc.__proto__"`), peopleTable,
			`"__proto__" is not allowed`},
		{"broken class row copied", replace(KindElement, "input", `"binding_mode": {"default_value": "read-write"}`, `"binding_mode": {"default_value": "write"}`), peopleTable,
			`class copy: "write" is not supported`},
		{"page in format 0.2", replace(KindPage, "form-page", `"schema_version": "0.5"`, `"schema_version": "0.2"`), peopleTable,
			`schema_version "0.2" is not supported`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fx := fixture()
			if c.edit != nil {
				c.edit(fx)
			}
			res, err := build(t, fx, c.tables)
			if err != nil {
				t.Fatal(err)
			}
			if res.Bundle != nil {
				t.Fatal("a bundle with errors must not be returned")
			}
			expectIssue(t, res.Errors, c.want)
		})
	}
}

// TestBuildBundlePaneLookup covers the outward pane lookup (OpenSpec
// wpi-pages-own-panes, design D9).
func TestBuildBundlePaneLookup(t *testing.T) {
	ok := func(name string, edit func(fx map[Kind]map[string]string)) {
		t.Run(name, func(t *testing.T) {
			fx := fixture()
			edit(fx)
			res, err := build(t, fx, peopleTable)
			if err != nil || len(res.Errors) != 0 {
				t.Fatalf("errors %+v, %v", res.Errors, err)
			}
		})
	}
	// The form page's root pane main is replaced by the pane it is opened in, so
	// main names right: the form opens the list in its own place.
	ok("root ID names the host pane", func(fx map[Kind]map[string]string) {
		fx[KindPage]["form-page"] = strings.Replace(fx[KindPage]["form-page"], `"pane": "left"`, `"pane": "main"`, 1)
	})
	// A multi-pane page opened in a pane: its own panes come first, then the host's.
	ok("multi-pane page in a pane", func(fx map[Kind]map[string]string) {
		fx[KindPage]["list-page"] = `{"descriptor_id": "list-page", "schema_version": "0.5",
		  "page_layouter": {"layouter": "single-pane", "root": {"pane_id": "split", "split": "rows", "children": [{"pane_id": "head"}, {"pane_id": "body"}]}},
		  "panels": {"head": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [{"element": "button", "label": "New",
		    "on_click": {"action": "open-page", "page": "form-page", "pane": "body", "input": {}}}]},
		    "body": {"elem_layouter": {"layouter": "flow-layouter"}, "elem_list": [{"element": "button", "label": "Edit",
		    "on_click": {"action": "open-page", "page": "form-page", "pane": "right", "input": {}}}]}}}`
		// Opened in body, the form's main names body; opened in right, main names right.
		fx[KindPage]["form-page"] = strings.Replace(fx[KindPage]["form-page"], `"pane": "left"`, `"pane": "main"`, 1)
	})
	t.Run("a pane no page in the chain has", func(t *testing.T) {
		fx := fixture()
		// The form is opened in home's pane right; neither it nor home has head.
		fx[KindPage]["form-page"] = strings.Replace(fx[KindPage]["form-page"], `"pane": "left"`, `"pane": "head"`, 1)
		res, err := build(t, fx, peopleTable)
		if err != nil {
			t.Fatal(err)
		}
		expectIssue(t, res.Errors, `pane "head" is not a leaf pane of page "form-page" or of a page it is opened in (form-page in home)`)
	})
}

func TestBuildBundleStoredRows(t *testing.T) {
	fx := fixture()
	fx[KindPage]["list-page"] = strings.Replace(fx[KindPage]["list-page"],
		`"source": {"action": "load", "table": "people", "result": "list", "fields": [{"column": "id", "attribute": "id"}, {"column": "name", "attribute": "name"}]}`,
		`"source": {"action": "query", "sql_stmt_id": "people-by-name", "result": "list"}`, 1)
	fx[KindPage]["form-page"] = strings.Replace(fx[KindPage]["form-page"], `"input": {}`, `"input": {"action": "load-file", "file_id": "new-person"}`, 1)
	columns := func(context.Context, string) (map[string]bool, error) {
		return map[string]bool{"id": true, "name": true}, nil
	}
	cases := []struct {
		name string
		rows map[string][]string
		want []string
	}{
		{"both registered", map[string][]string{storedStatement: {"people-by-name"}, storedFile: {"new-person"}}, nil},
		{"neither registered", nil, []string{
			`/panels/main/elem_list/0/source/sql_stmt_id SQL statement "people-by-name" is not registered in kb.stored_stmts`,
			`/input/file_id file "new-person" is not registered in kb.stored_files`}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := BuildBundle(context.Background(), "demo", fixtureLoader(t, fx), columns, registeredIn(c.rows))
			if err != nil {
				t.Fatal(err)
			}
			if len(res.Errors) != len(c.want) {
				t.Fatalf("errors %+v", res.Errors)
			}
			for _, w := range c.want {
				expectIssue(t, res.Errors, w)
			}
		})
	}
	t.Run("registry error", func(t *testing.T) {
		failing := func(context.Context, string, []string) (map[string]bool, error) { return nil, errors.New("db down") }
		if _, err := BuildBundle(context.Background(), "demo", fixtureLoader(t, fx), columns, failing); err == nil {
			t.Fatal("a registry error must fail the bundle")
		}
	})
}

func TestBuildBundleWarnsAboutUnboundSaveAttribute(t *testing.T) {
	fx := fixture()
	fx[KindPage]["form-page"] = strings.Replace(fx[KindPage]["form-page"], `"data_binding": "$input_doc.name"`, `"data_binding": "$input_doc.name", "binding_mode": "read-only"`, 1)
	res, err := build(t, fx, peopleTable)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || res.Bundle == nil {
		t.Fatalf("errors %+v", res.Errors)
	}
	expectIssue(t, res.Warnings, `writes attribute "name", but no read-write element on this page binds`)

	// A read-only page makes its inputs read-only too.
	fx = fixture()
	fx[KindPage]["form-page"] = strings.Replace(fx[KindPage]["form-page"], `"input": {}`, `"input": {}, "binding_mode": "read-only"`, 1)
	if res, err = build(t, fx, peopleTable); err != nil || len(res.Errors) != 0 {
		t.Fatalf("errors %+v, %v", res, err)
	}
	expectIssue(t, res.Warnings, `writes attribute "name", but no read-write element on this page binds`)
}

// A class that names an attribute its code module does not support still
// works: the attribute has no effect and the bundle carries a warning, once per
// class and page (OpenSpec wpi-code-modules, design D3).
func TestBuildBundleWarnsAboutUnsupportedAttribute(t *testing.T) {
	fx := fixture()
	fx[KindElement]["button"] = strings.Replace(fx[KindElement]["button"], `"on_click"`, `"label_hori_align": {"default_value": "center"}, "on_click"`, 1)
	fx[KindPage]["form-page"] = strings.Replace(fx[KindPage]["form-page"], `"label": "Save",`, `"label": "Save", "label_hori_align": "left",`, 1)
	res, err := build(t, fx, peopleTable)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || res.Bundle == nil {
		t.Fatalf("errors %+v", res.Errors)
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings %+v", res.Warnings)
	}
	expectIssue(t, res.Warnings, `/panels/main/elem_list/1/class/label_hori_align code module "button" does not support attribute "label_hori_align"`)
}

func TestBuildPagePreviewBundle(t *testing.T) {
	fx := fixture()
	fx[KindPage]["simple"] = `{"descriptor_id":"simple","schema_version":"0.5","page_layouter":{"layouter":"single-pane"},
	  "panels":{"main":{"elem_layouter":{"layouter":"flow-layouter"},"elem_list":[]}}}`
	result, err := BuildPagePreviewBundle(context.Background(), "simple", fixtureLoader(t, fx), func(context.Context, string) (map[string]bool, error) { return nil, nil }, registeredIn(nil))
	if err != nil || len(result.Errors) != 0 || result.Bundle == nil {
		t.Fatalf("preview bundle: %+v, %v", result, err)
	}
	if _, ok := result.Bundle.Pages["simple"]; !ok {
		t.Fatal("selected page missing")
	}
	// A page whose open-page names a pane only an outer page has fails on its own.
	if result, err = BuildPagePreviewBundle(context.Background(), "list-page", fixtureLoader(t, fx), func(context.Context, string) (map[string]bool, error) {
		return map[string]bool{"id": true, "name": true}, nil
	}, registeredIn(nil)); err != nil {
		t.Fatal(err)
	}
	expectIssue(t, result.Errors, `pane "right" is not a leaf pane of page "list-page"`)
}
