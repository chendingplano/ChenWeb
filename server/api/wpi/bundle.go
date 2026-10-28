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

// Loader returns the latest active descriptors of one kind among ids, or every
// active descriptor of that kind when ids is nil; IDs with no active version
// are absent from the map.
type Loader func(ctx context.Context, kind Kind, ids []string) (map[string]Descriptor, error)

// Columns returns a table's column names, or nil when the table does not exist.
type Columns func(ctx context.Context, table string) (map[string]bool, error)

// Registered returns which of names exist as stored rows of kind
// (storedStatement or storedFile).
type Registered func(ctx context.Context, kind string, names []string) (map[string]bool, error)

// BundleResult is a built bundle, or the errors that prevent building it.
type BundleResult struct {
	Bundle   *Bundle
	Errors   []Issue
	Warnings []Issue
}

// pageRef is a place inside a descriptor that names a page.
type pageRef struct {
	kind         Kind
	id, path, to string
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

// BuildBundle loads appID, every page reachable from it and all classes, then
// validates the whole set: structure, instances against their classes,
// references, panes, tables and columns, stored statements and files,
// bindings.
func BuildBundle(ctx context.Context, appID string, load Loader, columns Columns, registered Registered) (*BundleResult, error) {
	apps, err := load(ctx, KindApp, []string{appID})
	if err != nil {
		return nil, err
	}
	app, ok := apps[appID]
	if !ok {
		return nil, ErrNotFound
	}
	classes, err := loadClasses(ctx, load)
	if err != nil {
		return nil, err
	}
	pages, refs, err := loadPages(ctx, app, classes[KindAction], load)
	if err != nil {
		return nil, err
	}

	res := &BundleResult{}
	res.Errors = append(res.Errors, ValidateDescriptor(KindApp, appID, app.Body)...)
	for _, r := range refs {
		if _, ok := pages[r.to]; !ok {
			res.Errors = append(res.Errors, Issue{r.kind, r.id, r.path, fmt.Sprintf("page %q does not exist", r.to)})
		}
	}
	used := map[Kind]map[string]bool{KindElement: {}, KindAction: {}}
	var actions []mergedAction
	for _, id := range slices.Sorted(maps.Keys(pages)) {
		body := pages[id].Body
		if issues := ValidateDescriptor(KindPage, id, body); len(issues) > 0 {
			res.Errors = append(res.Errors, issues...)
			continue
		}
		v := &validator{kind: KindPage, id: id, classes: classes}
		els, acts := v.pageInstances(body)
		res.Errors = append(res.Errors, v.issues...)
		for _, e := range els {
			used[KindElement][e.cls] = true
		}
		for _, a := range acts {
			used[KindAction][a.cls] = true
		}
		actions = append(actions, acts...)
		res.Warnings = append(res.Warnings, unboundSaveWarnings(id, els, acts)...)
	}
	// Only the classes the app uses are checked, so a broken class row that no
	// page uses does not break every app. Attributes an element class names that
	// its code module does not support are warnings.
	usedElements := map[string]map[string]any{}
	for _, kind := range []Kind{KindElement, KindAction} {
		for _, cls := range slices.Sorted(maps.Keys(used[kind])) {
			res.Errors = append(res.Errors, ValidateDescriptor(kind, cls, classes[kind][cls].Body)...)
			if kind == KindElement {
				res.Warnings = append(res.Warnings, classWarnings(cls, classes[kind][cls].Body)...)
				usedElements[cls] = classes[kind][cls].Body
			}
		}
	}
	moduleErrs, err := moduleErrors(ctx, registered, usedElements)
	if err != nil {
		return nil, err
	}
	res.Errors = append(res.Errors, moduleErrs...)
	res.Errors = append(res.Errors, paneErrors(appID, str(app.Body["layout_template"]), actions)...)
	tableErrs, err := tableErrors(ctx, columns, actions)
	if err != nil {
		return nil, err
	}
	res.Errors = append(res.Errors, tableErrs...)
	storedErrs, err := storedErrors(ctx, registered, actions)
	if err != nil {
		return nil, err
	}
	res.Errors = append(res.Errors, storedErrs...)
	if len(res.Errors) > 0 {
		return res, nil
	}

	res.Bundle = &Bundle{
		App:      app.Raw,
		Pages:    rawMap(pages, nil),
		Elements: rawMap(classes[KindElement], used[KindElement]),
		Actions:  rawMap(classes[KindAction], used[KindAction]),
		Warnings: nonNil(res.Warnings),
	}
	return res, nil
}

// loadClasses loads every active element and action class. Classes are few by
// design, so all of them are loaded, one query per kind, and every instance can
// be merged as soon as its page arrives.
func loadClasses(ctx context.Context, load Loader) (map[Kind]map[string]Descriptor, error) {
	classes := map[Kind]map[string]Descriptor{}
	for _, kind := range []Kind{KindElement, KindAction} {
		var err error
		if classes[kind], err = load(ctx, kind, nil); err != nil {
			return nil, err
		}
	}
	return classes, nil
}

// ValidatePageInstances checks a page's instances against the active class
// rows, as a bundle does: classes exist, element instances set only what their
// class names and nothing it fixes, required attributes are set, and the merged
// instances are complete. Call it on a page that passes ValidateDescriptor.
func ValidatePageInstances(id string, page map[string]any, classes map[Kind]map[string]Descriptor) []Issue {
	v := &validator{kind: KindPage, id: id, classes: classes}
	v.pageInstances(page)
	return v.issues
}

// rawMap returns the descriptors' JSON by ID, only those in keep unless it is nil.
func rawMap(ds map[string]Descriptor, keep map[string]bool) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for id, d := range ds {
		if keep == nil || keep[id] {
			out[id] = d.Raw
		}
	}
	return out
}

// loadPages fetches every page reachable from the app round by round: the
// pages its panes name, then the pages their open-page instances open. Cycles
// end because each ID is tried once. It returns the pages found and every
// place that names a page.
func loadPages(ctx context.Context, app Descriptor, actionClasses map[string]Descriptor, load Loader) (map[string]Descriptor, []pageRef, error) {
	pages := map[string]Descriptor{}
	tried := map[string]bool{}
	var refs []pageRef
	var pending []string
	name := func(r pageRef) {
		if r.to == "" {
			return
		}
		refs = append(refs, r)
		if !tried[r.to] {
			tried[r.to] = true
			pending = append(pending, r.to)
		}
	}
	panes, _ := app.Body["panes"].(map[string]any)
	for _, pid := range slices.Sorted(maps.Keys(panes)) {
		p, _ := panes[pid].(map[string]any)
		name(pageRef{KindApp, app.ID, "/panes/" + pid + "/page", str(p["page"])})
	}
	for len(pending) > 0 {
		ids := slices.Sorted(slices.Values(pending))
		pending = nil
		found, err := load(ctx, KindPage, ids)
		if err != nil {
			return nil, nil, err
		}
		for _, id := range ids {
			d, ok := found[id]
			if !ok {
				continue
			}
			pages[id] = d
			for _, pos := range actionPositions(d.Body) {
				m, _ := pos.val.(map[string]any)
				if str(m["action"]) == "open-page" {
					merged := mergeInstance(KindAction, actionClasses["open-page"].Body, instanceAttrs(m, "action"))
					name(pageRef{KindPage, id, pos.path + "/page", str(merged["page"])})
				}
			}
		}
	}
	return pages, refs, nil
}

// unboundSaveWarnings warns about attributes a page's save_action writes that
// no read-write element on the page binds.
func unboundSaveWarnings(pageID string, els []mergedElement, acts []mergedAction) []Issue {
	bound := map[string]bool{}
	for _, e := range els {
		if str(e.body["binding_mode"]) == "read-write" {
			if ref, isRef, err := parseRef(str(e.body["data_binding"])); isRef && err == nil {
				bound[strings.Join(ref.Path, ".")] = true
			}
		}
	}
	var out []Issue
	for _, a := range acts {
		if a.path != "/save_action" {
			continue
		}
		fields, _ := a.body["fields"].([]any)
		for _, f := range fields {
			m, _ := f.(map[string]any)
			if attr := str(m["attribute"]); attr != "" && !bound[attr] {
				out = append(out, Issue{KindPage, pageID, "/save_action", fmt.Sprintf("the save action writes attribute %q, but no read-write element on this page binds $input_doc.%s", attr, attr)})
			}
		}
	}
	return out
}

func paneErrors(appID, tmpl string, actions []mergedAction) []Issue {
	panes, known := templatePanes[tmpl]
	if !known {
		return nil // reported by the app's structure check
	}
	var out []Issue
	for _, a := range actions {
		if pane := str(a.body["pane"]); a.cls == "open-page" && pane != "" && !slices.Contains(panes, pane) {
			out = append(out, Issue{KindPage, a.page, a.path + "/pane", fmt.Sprintf("pane %q is not part of layout template %q used by app %q", pane, tmpl, appID)})
		}
	}
	return out
}

func tableErrors(ctx context.Context, columns Columns, actions []mergedAction) ([]Issue, error) {
	cache := map[string]map[string]bool{}
	var out []Issue
	for _, a := range actions {
		table := str(a.body["table"])
		if (a.cls != "load" && a.cls != "save") || !tablePattern.MatchString(table) {
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
			out = append(out, Issue{KindPage, a.page, a.path + "/table", fmt.Sprintf("table %q does not exist", table)})
			continue
		}
		for _, c := range actionColumns(a.body) {
			if c.name != "" && !cols[c.name] {
				out = append(out, Issue{KindPage, a.page, a.path + c.path, fmt.Sprintf("column %q does not exist in table %q", c.name, table)})
			}
		}
	}
	return out, nil
}

// moduleErrors reports element class rows whose code_module_id does not name an
// active element module in kb.code_modules (OpenSpec wpi-code-modules, design
// D9). A missing or malformed code_module_id is left to the class checks.
func moduleErrors(ctx context.Context, registered Registered, rows map[string]map[string]any) ([]Issue, error) {
	var names []string
	for _, id := range slices.Sorted(maps.Keys(rows)) {
		if m := str(rows[id]["code_module_id"]); idPattern.MatchString(m) {
			names = append(names, m)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	found, err := registered(ctx, storedModule, names)
	if err != nil {
		return nil, err
	}
	var out []Issue
	for _, id := range slices.Sorted(maps.Keys(rows)) {
		if m := str(rows[id]["code_module_id"]); idPattern.MatchString(m) && !found[m] {
			out = append(out, Issue{KindElement, id, "/code_module_id", fmt.Sprintf("code module %q is not an active element module in kb.code_modules", m)})
		}
	}
	return out, nil
}

// storedRefs says which attribute of a query or load-file instance names a
// stored row, of which kind.
var storedRefs = map[string]struct{ kind, attr, what string }{
	"query":     {storedStatement, "sql_stmt_id", "SQL statement %q is not registered in kb.stored_stmts"},
	"load-file": {storedFile, "file_id", "file %q is not registered in kb.stored_files"},
}

// storedErrors reports query and load-file instances whose statement or file
// does not exist (design D9). Access is checked when an instance runs.
func storedErrors(ctx context.Context, registered Registered, actions []mergedAction) ([]Issue, error) {
	names := map[string][]string{}
	for _, a := range actions {
		if r, ok := storedRefs[a.cls]; ok && idPattern.MatchString(str(a.body[r.attr])) {
			names[r.kind] = append(names[r.kind], str(a.body[r.attr]))
		}
	}
	found := map[string]map[string]bool{}
	for _, kind := range slices.Sorted(maps.Keys(names)) {
		var err error
		if found[kind], err = registered(ctx, kind, names[kind]); err != nil {
			return nil, err
		}
	}
	var out []Issue
	for _, a := range actions {
		r, ok := storedRefs[a.cls]
		if name := str(a.body[r.attr]); ok && idPattern.MatchString(name) && !found[r.kind][name] {
			out = append(out, Issue{KindPage, a.page, a.path + "/" + r.attr, fmt.Sprintf(r.what, name)})
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
