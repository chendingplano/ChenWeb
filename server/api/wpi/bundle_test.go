package wpi

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
)

// fixture is a small app: a list page and a form page that open each other,
// so the page walk has to stop at a cycle.
func fixture() map[Kind]map[string]string {
	fx := testClasses()
	fx[KindApp] = map[string]string{
		"demo": `{"descriptor_id": "demo", "schema_version": "0.2", "layout_template": "top-left-middle-right-bottom",
		  "panes": {"left": {"page": "list-page"}, "right": {}}}`,
	}
	fx[KindPage] = map[string]string{
		"list-page": `{"descriptor_id": "list-page", "schema_version": "0.2", "layouter": "flow-layouter",
		  "elements": [{"element": "list",
		    "source": {"action": "load", "table": "people", "result": "list", "fields": [{"column": "id", "attribute": "id"}, {"column": "name", "attribute": "name"}]},
		    "columns": [{"field": "name", "label": "Name"}],
		    "on_row_click": {"action": "open-page", "page": "form-page", "pane": "right", "args": {"id": "$record.id"},
		      "input": {"action": "load", "table": "people", "result": "record", "where": {"id": "$args.id"},
		        "fields": [{"column": "id", "attribute": "id"}, {"column": "name", "attribute": "name"}]}}}]}`,
		"form-page": `{"descriptor_id": "form-page", "schema_version": "0.2", "layouter": "flow-layouter",
		  "input": {}, "save_action": {"action": "save", "table": "people", "fields": [{"attribute": "name", "column": "name", "required": true}]},
		  "elements": [{"element": "input", "label": "Name", "data_binding": "$input_doc.name"},
		    {"element": "button", "label": "Save", "on_click": {"action": "save-page"}},
		    {"element": "button", "label": "Back", "on_click": {"action": "open-page", "page": "list-page", "pane": "left"}}]}`,
	}
	return fx
}

func fixtureLoader(t *testing.T, fx map[Kind]map[string]string) Loader {
	return func(_ context.Context, kind Kind, ids []string) (map[string]Descriptor, error) {
		out := map[string]Descriptor{}
		for id, s := range fx[kind] {
			if ids == nil || slices.Contains(ids, id) {
				out[id] = Descriptor{Kind: kind, ID: id, Version: 1, Raw: []byte(s), Body: decode(t, s)}
			}
		}
		return out, nil
	}
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
// element modules of kb.code_modules: every module with code, unless rows names
// them.
func registeredIn(rows map[string][]string) Registered {
	return func(_ context.Context, kind string, names []string) (map[string]bool, error) {
		known, given := rows[kind]
		if kind == storedModule && !given {
			known = slices.Collect(maps.Keys(codeModules))
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

// A class whose code module is suspended, or has no row in kb.code_modules,
// stops the apps that use it (OpenSpec wpi-code-modules, design D9).
func TestBuildBundleNeedsActiveCodeModules(t *testing.T) {
	columns := func(_ context.Context, table string) (map[string]bool, error) {
		return map[string]bool{"id": true, "name": true}, nil
	}
	res, err := BuildBundle(context.Background(), "demo", fixtureLoader(t, fixture()), columns,
		registeredIn(map[string][]string{storedModule: {"input", "list"}}))
	if err != nil {
		t.Fatal(err)
	}
	if res.Bundle != nil {
		t.Fatal("a bundle with a suspended module must not be built")
	}
	expectIssue(t, res.Errors, `/code_module_id code module "button" is not an active element module in kb.code_modules`)
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
	// Only the classes the app uses: list, input, button; load, open-page, save, save-page.
	if len(b.Pages) != 2 || len(b.Elements) != 3 || len(b.Actions) != 4 {
		t.Fatalf("pages %d elements %d actions %d", len(b.Pages), len(b.Elements), len(b.Actions))
	}
	if _, ok := b.Elements["menu"]; ok {
		t.Fatal("unused classes must not be in the bundle")
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
		{"missing class row", func(fx map[Kind]map[string]string) { delete(fx[KindElement], "input") }, peopleTable,
			`element class "input" does not exist`},
		{"missing page", replace(KindPage, "list-page", `"page": "form-page"`, `"page": "nowhere"`), peopleTable,
			`/elements/0/on_row_click/page page "nowhere" does not exist`},
		{"unknown pane", replace(KindPage, "list-page", `"pane": "right"`, `"pane": "side"`), peopleTable,
			`pane "side" is not part of layout template`},
		{"open-page in a button to an unknown pane", replace(KindPage, "form-page", `"pane": "left"`, `"pane": "main"`), peopleTable,
			`/elements/2/on_click/pane pane "main" is not part of layout template`},
		{"missing column", nil, map[string][]string{"people": {"id"}}, `column "name" does not exist in table "people"`},
		{"missing table", nil, map[string][]string{}, `table "people" does not exist`},
		{"wrong class in a position", replace(KindPage, "list-page", `"source": {"action": "load"`, `"source": {"action": "save"`), peopleTable,
			"a save action is not allowed here (allowed: load, query)"},
		{"null attribute not set", replace(KindPage, "form-page", `"label": "Name", `, ``), peopleTable,
			`attribute "label" has no default in element class "input"`},
		{"structure error in a reached page", replace(KindPage, "form-page", `"$input_doc.name"`, `"$input_doc.__proto__"`), peopleTable,
			`"__proto__" is not allowed`},
		{"broken class row in use", replace(KindElement, "input", `"binding_mode": {"default_value": "read-write"}`, `"binding_mode": {"default_value": "write"}`), peopleTable,
			`"write" is not supported`},
		{"page in format 0.1", replace(KindPage, "form-page", `"schema_version": "0.2"`, `"schema_version": "0.1"`), peopleTable,
			`schema_version "0.1" is not supported`},
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
			`/elements/0/source/sql_stmt_id SQL statement "people-by-name" is not registered in kb.stored_stmts`,
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
// works: the attribute has no effect and the bundle carries a warning
// (OpenSpec wpi-code-modules, design D3).
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
	expectIssue(t, res.Warnings, `/label_hori_align code module "button" does not support attribute "label_hori_align"`)
}

func TestBuildPagePreviewBundle(t *testing.T) {
	fx := fixture()
	fx[KindPage]["simple"] = `{"descriptor_id":"simple","schema_version":"0.2","layouter":"flow-layouter","elements":[]}`
	result, err := BuildPagePreviewBundle(context.Background(), "simple", fixtureLoader(t, fx), func(context.Context, string) (map[string]bool, error) { return nil, nil }, registeredIn(nil))
	if err != nil || len(result.Errors) != 0 || result.Bundle == nil {
		t.Fatalf("preview bundle: %+v, %v", result, err)
	}
	if _, ok := result.Bundle.Pages["simple"]; !ok {
		t.Fatal("selected page missing")
	}
}
