package wpi

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fixture is a small app: a list page and a form page that open each other,
// so the reference walk has to stop at a cycle.
func fixture() map[Kind]map[string]string {
	return map[Kind]map[string]string{
		KindApp: {
			"demo": `{"descriptor_id": "demo", "schema_version": "0.1", "layout_template": "top-left-middle-right-bottom",
			  "panes": {"left": {"page": "list-page"}, "right": {}}}`,
		},
		KindPage: {
			"list-page": `{"descriptor_id": "list-page", "schema_version": "0.1", "layouter": "flow-layouter",
			  "elements": [{"descriptor_id": "people", "element_type": "list", "source": {"action": "load-people"},
			    "presentation": "rows", "columns": [{"field": "name", "label": "Name"}],
			    "on_row_click": {"action": "open-person", "args": {"id": "$record.id"}}}]}`,
			"form-page": `{"descriptor_id": "form-page", "schema_version": "0.1", "layouter": "flow-layouter",
			  "input": {}, "save_action": "save-person",
			  "elements": [{"element": "name-input"},
			    {"descriptor_id": "back", "element_type": "button", "label": "Back",
			     "on_click": {"action_type": "open_page", "page": "list-page", "pane": "left"}}]}`,
		},
		KindElement: {
			"name-input": `{"descriptor_id": "name-input", "schema_version": "0.1", "element_type": "input",
			  "label": "Name", "data_binding": "$input_doc.name", "binding_mode": "read-write"}`,
		},
		KindAction: {
			"load-people": `{"descriptor_id": "load-people", "schema_version": "0.1", "action_type": "load",
			  "table": "people", "result": "list", "fields": [{"column": "id", "attribute": "id"}, {"column": "name", "attribute": "name"}]}`,
			"load-person": `{"descriptor_id": "load-person", "schema_version": "0.1", "action_type": "load",
			  "table": "people", "result": "record", "where": {"id": "$args.id"}, "fields": [{"column": "id", "attribute": "id"}, {"column": "name", "attribute": "name"}]}`,
			"open-person": `{"descriptor_id": "open-person", "schema_version": "0.1", "action_type": "open_page",
			  "page": "form-page", "pane": "right", "input": {"action": "load-person", "args": {"id": "$args.id"}}}`,
			"save-person": `{"descriptor_id": "save-person", "schema_version": "0.1", "action_type": "save",
			  "table": "people", "mode": "upsert", "key": "id", "fields": [{"attribute": "name", "column": "name", "required": true}]}`,
		},
	}
}

func build(t *testing.T, fx map[Kind]map[string]string, tables map[string][]string) (*BundleResult, error) {
	t.Helper()
	load := func(_ context.Context, kind Kind, ids []string) (map[string]Descriptor, error) {
		out := map[string]Descriptor{}
		for _, id := range ids {
			if s, ok := fx[kind][id]; ok {
				out[id] = Descriptor{Kind: kind, ID: id, Version: 1, Raw: []byte(s), Body: decode(t, s)}
			}
		}
		return out, nil
	}
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
	return BuildBundle(context.Background(), "demo", load, columns)
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
	if len(b.Pages) != 2 || len(b.Elements) != 1 || len(b.Actions) != 4 {
		t.Fatalf("pages %d elements %d actions %d", len(b.Pages), len(b.Elements), len(b.Actions))
	}
}

func TestBuildBundleNotFound(t *testing.T) {
	fx := fixture()
	delete(fx[KindApp], "demo")
	if _, err := build(t, fx, peopleTable); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestBuildBundleErrors(t *testing.T) {
	cases := []struct {
		name   string
		edit   func(fx map[Kind]map[string]string)
		tables map[string][]string
		want   string
	}{
		{"missing element", func(fx map[Kind]map[string]string) { delete(fx[KindElement], "name-input") }, peopleTable,
			`element "name-input" does not exist`},
		{"unknown pane", func(fx map[Kind]map[string]string) {
			fx[KindAction]["open-person"] = strings.Replace(fx[KindAction]["open-person"], `"pane": "right"`, `"pane": "side"`, 1)
		}, peopleTable, `pane "side" is not part of layout template`},
		{"inline open_page to unknown pane", func(fx map[Kind]map[string]string) {
			fx[KindPage]["form-page"] = strings.Replace(fx[KindPage]["form-page"], `"pane": "left"`, `"pane": "main"`, 1)
		}, peopleTable, `pane "main" is not part of layout template`},
		{"missing column", nil, map[string][]string{"people": {"id"}}, `column "name" does not exist in table "people"`},
		{"missing table", nil, map[string][]string{}, `table "people" does not exist`},
		{"wrong action type", func(fx map[Kind]map[string]string) {
			fx[KindPage]["list-page"] = strings.Replace(fx[KindPage]["list-page"], `"action": "load-people"`, `"action": "save-person"`, 1)
		}, peopleTable, `action "save-person" is a save action; expected load`},
		{"override checked against type", func(fx map[Kind]map[string]string) {
			fx[KindPage]["form-page"] = strings.Replace(fx[KindPage]["form-page"], `{"element": "name-input"}`, `{"element": "name-input", "binding_mode": "write"}`, 1)
		}, peopleTable, `"write" is not supported`},
		{"load in a trigger", func(fx map[Kind]map[string]string) {
			fx[KindPage]["form-page"] = strings.Replace(fx[KindPage]["form-page"], `{"action_type": "open_page", "page": "list-page", "pane": "left"}`,
				`{"action_type": "load", "table": "people", "result": "list", "fields": [{"column": "id", "attribute": "id"}]}`, 1)
		}, peopleTable, "a load action cannot be triggered by an event"},
		{"structure error in a reached descriptor", func(fx map[Kind]map[string]string) {
			fx[KindElement]["name-input"] = strings.Replace(fx[KindElement]["name-input"], `"$input_doc.name"`, `"$input_doc.__proto__"`, 1)
		}, peopleTable, `"__proto__" is not allowed`},
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
	fx[KindElement]["name-input"] = strings.Replace(fx[KindElement]["name-input"], `"read-write"`, `"read-only"`, 1)
	res, err := build(t, fx, peopleTable)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 0 || res.Bundle == nil {
		t.Fatalf("errors %+v", res.Errors)
	}
	expectIssue(t, res.Warnings, `writes attribute "name", but no read-write element on this page binds`)
}
