package wpi

import (
	"context"
	"errors"
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
	return BuildBundle(context.Background(), "demo", fixtureLoader(t, fx), columns)
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
	fx[KindElement]["select"] = `{"descriptor_id": "select", "schema_version": "0.2"}`
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
			"a save action is not allowed here (allowed: load)"},
		{"null attribute not set", replace(KindPage, "form-page", `"label": "Name", `, ``), peopleTable,
			`attribute "label" has no default in element class "input"`},
		{"structure error in a reached page", replace(KindPage, "form-page", `"$input_doc.name"`, `"$input_doc.__proto__"`), peopleTable,
			`"__proto__" is not allowed`},
		{"broken class row in use", replace(KindElement, "input", `"binding_mode": "read-write"`, `"binding_mode": "write"`), peopleTable,
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
}
