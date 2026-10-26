package wpi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
)

// ErrNotFound means the requested app (or record) does not exist.
var ErrNotFound = errors.New("not found")

// Loader returns the latest active descriptors of one kind among ids; IDs
// with no active version are absent from the map.
type Loader func(ctx context.Context, kind Kind, ids []string) (map[string]Descriptor, error)

// Columns returns a table's column names, or nil when the table does not exist.
type Columns func(ctx context.Context, table string) (map[string]bool, error)

// BundleResult is a built bundle, or the errors that prevent building it.
type BundleResult struct {
	Bundle   *Bundle
	Errors   []Issue
	Warnings []Issue
}

// site is a place inside a descriptor that refers to another descriptor.
// expect lists the action types allowed there (nil: any).
type site struct {
	kind   Kind
	id     string
	path   string
	expect []string
}

// actionSite is an action body reachable from the app, stored or inline.
type actionSite struct {
	kind Kind
	id   string
	path string
	body map[string]any
}

type walker struct {
	got     map[Kind]map[string]Descriptor
	tried   map[Kind]map[string]bool
	sites   map[Kind]map[string][]site
	actions []actionSite
	issues  []Issue
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// BuildBundle loads appID and everything reachable from it, then validates the
// whole set: structure, references, panes, tables and columns, bindings.
func BuildBundle(ctx context.Context, appID string, load Loader, columns Columns) (*BundleResult, error) {
	apps, err := load(ctx, KindApp, []string{appID})
	if err != nil {
		return nil, err
	}
	app, ok := apps[appID]
	if !ok {
		return nil, ErrNotFound
	}
	w := &walker{got: map[Kind]map[string]Descriptor{}, tried: map[Kind]map[string]bool{}, sites: map[Kind]map[string][]site{}}
	for _, k := range []Kind{KindApp, KindPage, KindElement, KindAction} {
		w.got[k], w.tried[k], w.sites[k] = map[string]Descriptor{}, map[string]bool{}, map[string][]site{}
	}
	w.add(app)
	// Fetch referenced descriptors round by round; cycles end because each
	// ID is tried once.
	for {
		fetched := false
		for _, kind := range []Kind{KindPage, KindElement, KindAction} {
			var ids []string
			for _, id := range slices.Sorted(maps.Keys(w.sites[kind])) {
				if !w.tried[kind][id] {
					ids = append(ids, id)
				}
			}
			if len(ids) == 0 {
				continue
			}
			fetched = true
			found, err := load(ctx, kind, ids)
			if err != nil {
				return nil, err
			}
			for _, id := range ids {
				w.tried[kind][id] = true
				if d, ok := found[id]; ok {
					w.add(d)
				}
			}
		}
		if !fetched {
			break
		}
	}

	res := &BundleResult{}
	for _, kind := range []Kind{KindApp, KindPage, KindElement, KindAction} {
		for _, id := range slices.Sorted(maps.Keys(w.got[kind])) {
			res.Errors = append(res.Errors, ValidateDescriptor(kind, id, w.got[kind][id].Body)...)
		}
	}
	res.Errors = append(res.Errors, w.issues...)
	res.Errors = append(res.Errors, w.referenceErrors()...)
	pageErrs, warnings := w.pageChecks()
	res.Errors = append(res.Errors, pageErrs...)
	res.Warnings = warnings
	res.Errors = append(res.Errors, w.paneErrors(appID, str(app.Body["layout_template"]))...)
	tableErrs, err := w.tableErrors(ctx, columns)
	if err != nil {
		return nil, err
	}
	res.Errors = append(res.Errors, tableErrs...)
	if len(res.Errors) > 0 {
		return res, nil
	}

	b := &Bundle{App: app.Raw, Pages: rawMap(w.got[KindPage]), Elements: rawMap(w.got[KindElement]), Actions: rawMap(w.got[KindAction]), Warnings: res.Warnings}
	if b.Warnings == nil {
		b.Warnings = []Issue{}
	}
	res.Bundle = b
	return res, nil
}

func rawMap(ds map[string]Descriptor) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(ds))
	for id, d := range ds {
		out[id] = d.Raw
	}
	return out
}

func (w *walker) add(d Descriptor) {
	w.got[d.Kind][d.ID] = d
	b := d.Body
	switch d.Kind {
	case KindApp:
		panes, _ := b["panes"].(map[string]any)
		for _, pid := range slices.Sorted(maps.Keys(panes)) {
			p, _ := panes[pid].(map[string]any)
			w.ref(KindPage, str(p["page"]), site{KindApp, d.ID, "/panes/" + pid + "/page", nil})
		}
	case KindPage:
		w.ref(KindAction, str(b["save_action"]), site{KindPage, d.ID, "/save_action", []string{"save"}})
		w.input(b["input"], KindPage, d.ID, "/input")
		els, _ := b["elements"].([]any)
		for i, e := range els {
			m, _ := e.(map[string]any)
			path := "/elements/" + strconv.Itoa(i)
			w.ref(KindElement, str(m["element"]), site{KindPage, d.ID, path + "/element", nil})
			w.element(m, KindPage, d.ID, path)
		}
	case KindElement:
		w.element(b, KindElement, d.ID, "")
	case KindAction:
		w.action(b, KindAction, d.ID, "")
	}
}

func (w *walker) ref(kind Kind, id string, s site) {
	if id != "" {
		w.sites[kind][id] = append(w.sites[kind][id], s)
	}
}

func (w *walker) input(val any, kind Kind, id, path string) {
	m, _ := val.(map[string]any)
	w.ref(KindAction, str(m["action"]), site{kind, id, path + "/action", []string{"load"}})
}

func (w *walker) element(m map[string]any, kind Kind, id, path string) {
	if src, ok := m["source"].(map[string]any); ok {
		w.ref(KindAction, str(src["action"]), site{kind, id, path + "/source/action", []string{"load"}})
	}
	w.trigger(m["on_click"], kind, id, path+"/on_click")
	w.trigger(m["on_row_click"], kind, id, path+"/on_row_click")
	w.items(m["items"], kind, id, path+"/items")
}

func (w *walker) items(val any, kind Kind, id, path string) {
	list, _ := val.([]any)
	for i, it := range list {
		m, _ := it.(map[string]any)
		p := path + "/" + strconv.Itoa(i)
		w.trigger(m["on_click"], kind, id, p+"/on_click")
		w.items(m["children"], kind, id, p+"/children")
	}
}

var triggerActionTypes = []string{"save", "open_page", "close_page"}

func (w *walker) trigger(val any, kind Kind, id, path string) {
	t, ok := val.(map[string]any)
	if !ok {
		return
	}
	if _, byID := t["action"]; byID {
		w.ref(KindAction, str(t["action"]), site{kind, id, path + "/action", triggerActionTypes})
		return
	}
	if typ := str(t["action_type"]); typ != "" && !slices.Contains(triggerActionTypes, typ) {
		w.issues = append(w.issues, Issue{kind, id, path + "/action_type", fmt.Sprintf("a %s action cannot be triggered by an event (allowed: %v)", typ, triggerActionTypes)})
	}
	w.action(t, kind, id, path)
}

func (w *walker) action(a map[string]any, kind Kind, id, path string) {
	w.actions = append(w.actions, actionSite{kind, id, path, a})
	if str(a["action_type"]) == "open_page" {
		w.ref(KindPage, str(a["page"]), site{kind, id, path + "/page", nil})
		w.input(a["input"], kind, id, path+"/input")
	}
}

func (w *walker) referenceErrors() []Issue {
	var out []Issue
	for _, kind := range []Kind{KindPage, KindElement, KindAction} {
		for _, target := range slices.Sorted(maps.Keys(w.sites[kind])) {
			d, exists := w.got[kind][target]
			for _, s := range w.sites[kind][target] {
				if !exists {
					out = append(out, Issue{s.kind, s.id, s.path, fmt.Sprintf("%s %q does not exist", kind, target)})
					continue
				}
				if typ := str(d.Body["action_type"]); s.expect != nil && !slices.Contains(s.expect, typ) {
					out = append(out, Issue{s.kind, s.id, s.path, fmt.Sprintf("action %q is a %s action; expected %s", target, typ, strings.Join(s.expect, " or "))})
				}
			}
		}
	}
	return out
}

// pageChecks validates element entries with overrides against their stored
// element's type, and warns about save attributes no read-write element binds.
func (w *walker) pageChecks() (errs, warnings []Issue) {
	elementOK := map[string]bool{}
	for id, d := range w.got[KindElement] {
		elementOK[id] = len(ValidateDescriptor(KindElement, id, d.Body)) == 0
	}
	for _, pageID := range slices.Sorted(maps.Keys(w.got[KindPage])) {
		page := w.got[KindPage][pageID].Body
		bound := map[string]bool{}
		els, _ := page["elements"].([]any)
		for i, e := range els {
			m, _ := e.(map[string]any)
			el := m
			if ref := str(m["element"]); ref != "" {
				stored, ok := w.got[KindElement][ref]
				if !ok || !elementOK[ref] {
					continue
				}
				el = maps.Clone(stored.Body)
				for k, v := range m {
					if k != "element" {
						el[k] = v
					}
				}
				if len(m) > 1 {
					v := &validator{kind: KindPage, id: pageID}
					v.element(el, "/elements/"+strconv.Itoa(i), true)
					errs = append(errs, v.issues...)
				}
			}
			if str(el["binding_mode"]) == "read-write" {
				if ref, isRef, err := parseRef(str(el["data_binding"])); isRef && err == nil {
					bound[strings.Join(ref.Path, ".")] = true
				}
			}
		}
		save, ok := w.got[KindAction][str(page["save_action"])]
		if !ok || str(save.Body["action_type"]) != "save" {
			continue
		}
		fields, _ := save.Body["fields"].([]any)
		for _, f := range fields {
			m, _ := f.(map[string]any)
			attr := str(m["attribute"])
			if attr != "" && !bound[attr] {
				warnings = append(warnings, Issue{KindPage, pageID, "/save_action", fmt.Sprintf("save action %q writes attribute %q, but no read-write element on this page binds $input_doc.%s", save.ID, attr, attr)})
			}
		}
	}
	return errs, warnings
}

func (w *walker) paneErrors(appID, tmpl string) []Issue {
	panes, known := templatePanes[tmpl]
	if !known {
		return nil // reported by the app's structure check
	}
	var out []Issue
	for _, a := range w.actions {
		if str(a.body["action_type"]) != "open_page" {
			continue
		}
		if pane := str(a.body["pane"]); pane != "" && !slices.Contains(panes, pane) {
			out = append(out, Issue{a.kind, a.id, a.path + "/pane", fmt.Sprintf("pane %q is not part of layout template %q used by app %q", pane, tmpl, appID)})
		}
	}
	return out
}

func (w *walker) tableErrors(ctx context.Context, columns Columns) ([]Issue, error) {
	cache := map[string]map[string]bool{}
	var out []Issue
	for _, a := range w.actions {
		typ := str(a.body["action_type"])
		table := str(a.body["table"])
		if (typ != "load" && typ != "save") || !tablePattern.MatchString(table) {
			continue
		}
		cols, seen := cache[table]
		if !seen {
			var err error
			if cols, err = columns(ctx, table); err != nil {
				return nil, err
			}
			cache[table] = cols
		}
		if cols == nil {
			out = append(out, Issue{a.kind, a.id, a.path + "/table", fmt.Sprintf("table %q does not exist", table)})
			continue
		}
		for _, c := range actionColumns(a.body) {
			if c.name != "" && !cols[c.name] {
				out = append(out, Issue{a.kind, a.id, a.path + c.path, fmt.Sprintf("column %q does not exist in table %q", c.name, table)})
			}
		}
	}
	return out, nil
}

type namedColumn struct{ name, path string }

// actionColumns lists every column a load or save action names.
func actionColumns(a map[string]any) []namedColumn {
	var out []namedColumn
	fields, _ := a["fields"].([]any)
	for i, f := range fields {
		m, _ := f.(map[string]any)
		out = append(out, namedColumn{str(m["column"]), "/fields/" + strconv.Itoa(i) + "/column"})
	}
	where, _ := a["where"].(map[string]any)
	for _, c := range slices.Sorted(maps.Keys(where)) {
		out = append(out, namedColumn{c, "/where/" + c})
	}
	order, _ := a["order_by"].([]any)
	for i, o := range order {
		m, _ := o.(map[string]any)
		out = append(out, namedColumn{str(m["column"]), "/order_by/" + strconv.Itoa(i) + "/column"})
	}
	if k := str(a["key"]); k != "" {
		out = append(out, namedColumn{k, "/key"})
	}
	return out
}
