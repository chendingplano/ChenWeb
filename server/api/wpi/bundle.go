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

// BuildBundle loads appID and every page reachable from it, then validates the
// whole set: each page with its instances against their class copies, the
// code modules the copies name, references, the panes open-page instances name,
// tables and columns, stored statements and files, bindings. Instances carry
// their class copies, so no class rows are loaded (OpenSpec wpi-pages-own-panes).
func BuildBundle(ctx context.Context, appID string, load Loader, columns Columns, registered Registered) (*BundleResult, error) {
	apps, err := load(ctx, KindApp, []string{appID})
	if err != nil {
		return nil, err
	}
	app, ok := apps[appID]
	if !ok {
		return nil, ErrNotFound
	}
	initial := str(app.Body["initial_page"])
	pages, refs, err := loadPages(ctx, appID, initial, load)
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
	var actions []mergedAction
	byPage := map[string][]mergedAction{}
	var modules []moduleRef
	for _, id := range slices.Sorted(maps.Keys(pages)) {
		body := pages[id].Body
		issues, els, acts := validatePage(id, body)
		if len(issues) > 0 {
			res.Errors = append(res.Errors, issues...)
			continue
		}
		actions = append(actions, acts...)
		byPage[id] = acts
		modules = append(modules, pageModules(id, body, els)...)
		res.Warnings = append(res.Warnings, unboundSaveWarnings(id, els, acts)...)
		res.Warnings = append(res.Warnings, copyWarnings(id, els)...)
		res.Warnings = append(res.Warnings, chatWarnings(id, els)...)
	}
	moduleErrs, err := moduleErrors(ctx, registered, modules)
	if err != nil {
		return nil, err
	}
	res.Errors = append(res.Errors, moduleErrs...)
	res.Errors = append(res.Errors, paneErrors(initial, pages, byPage)...)
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
		Pages:    rawMap(pages),
		Warnings: nonNil(res.Warnings),
	}
	return res, nil
}

// BuildPagePreviewBundle builds an app whose initial page is a stored page.
func BuildPagePreviewBundle(ctx context.Context, pageID string, load Loader, columns Columns, registered Registered) (*BundleResult, error) {
	pages, err := load(ctx, KindPage, []string{pageID})
	if err != nil {
		return nil, err
	}
	if _, ok := pages[pageID]; !ok {
		return nil, ErrNotFound
	}
	body := map[string]any{"descriptor_id": "editor-preview", "schema_version": SchemaVersion, "initial_page": pageID}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	previewLoader := func(ctx context.Context, kind Kind, ids []string) (map[string]Descriptor, error) {
		if kind == KindApp {
			return map[string]Descriptor{"editor-preview": {Kind: KindApp, ID: "editor-preview", Raw: raw, Body: body}}, nil
		}
		return load(ctx, kind, ids)
	}
	return BuildBundle(ctx, "editor-preview", previewLoader, columns, registered)
}

// BuildDraftPagePreviewBundle validates a temporary page without storing it.
func BuildDraftPagePreviewBundle(ctx context.Context, body map[string]any, load Loader, columns Columns, registered Registered) (*BundleResult, error) {
	id := str(body["descriptor_id"])
	if id == "" {
		id = "editor-preview-page"
		body["descriptor_id"] = id
	}
	issues, err := fillClassCopies(ctx, id, body, load)
	if err != nil {
		return nil, err
	}
	if len(issues) > 0 {
		return &BundleResult{Errors: issues}, nil
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	draft := Descriptor{Kind: KindPage, ID: id, Raw: raw, Body: body}
	draftLoader := func(ctx context.Context, kind Kind, ids []string) (map[string]Descriptor, error) {
		if kind != KindPage {
			return load(ctx, kind, ids)
		}
		others := make([]string, 0, len(ids))
		for _, requested := range ids {
			if requested != id {
				others = append(others, requested)
			}
		}
		rows := map[string]Descriptor{}
		if len(others) > 0 {
			var err error
			rows, err = load(ctx, kind, others)
			if err != nil {
				return nil, err
			}
		}
		for _, requested := range ids {
			if requested == id {
				rows[id] = draft
			}
		}
		return rows, nil
	}
	return BuildPagePreviewBundle(ctx, id, draftLoader, columns, registered)
}

// rawMap returns the descriptors' JSON by ID.
func rawMap(ds map[string]Descriptor) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	for id, d := range ds {
		out[id] = d.Raw
	}
	return out
}

// loadPages fetches every page reachable from the app round by round: its
// initial page, then the pages its open-page instances open. Cycles end because
// each ID is tried once. It returns the pages found and every place that names
// a page.
func loadPages(ctx context.Context, appID, initial string, load Loader) (map[string]Descriptor, []pageRef, error) {
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
	name(pageRef{KindApp, appID, "/initial_page", initial})
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
					row, _ := m[copyKey].(map[string]any)
					merged := mergeInstance(KindAction, row, instanceAttrs(m, "action"))
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

// maxHostChain caps how many pages a page can be opened inside, one in the other,
// when the bundle checks panes, so cycles of open-page instances end.
const maxHostChain = 8

// paneErrors checks that the pane of every open-page instance resolves, looking
// outward from the page that holds it through the pages it can be opened in
// (devdoc 2026102601 §5.1, OpenSpec wpi-pages-own-panes design D9). It walks
// (page, host chain) pairs from the initial page, which has no host; a page
// opened in a pane gets the chain of that pane's page. acts holds each valid
// page's merged action instances.
func paneErrors(initial string, pages map[string]Descriptor, acts map[string][]mergedAction) []Issue {
	type visit struct {
		page  string
		chain []string // host pages, innermost first
	}
	seen := map[string]bool{}
	reported := map[string]bool{}
	var out []Issue
	queue := []visit{{initial, nil}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		key := cur.page + "|" + strings.Join(cur.chain, "/")
		if seen[key] || len(cur.chain) > maxHostChain {
			continue
		}
		seen[key] = true
		full := append([]string{cur.page}, cur.chain...)
		for _, a := range acts[cur.page] {
			target, pane := str(a.body["page"]), str(a.body["pane"])
			if a.cls != "open-page" || pane == "" {
				continue
			}
			chain, ok := resolvePane(pages, full, pane)
			if !ok {
				msg := fmt.Sprintf("pane %q is not a leaf pane of page %q or of a page it is opened in (%s)", pane, cur.page, strings.Join(full, " in "))
				if !reported[a.path+msg] {
					reported[a.path+msg] = true
					out = append(out, Issue{KindPage, cur.page, a.path + "/pane", msg})
				}
				continue
			}
			if _, exists := pages[target]; exists {
				queue = append(queue, visit{target, chain})
			}
		}
	}
	return out
}

// resolvePane looks pane up in full, a page followed by the pages it is opened
// in, innermost first. In each page it accepts a leaf pane; a page opened in a
// pane has that pane in place of its root, so its root's ID names the pane of
// the next page. It returns the host chain of a page opened in the pane found.
func resolvePane(pages map[string]Descriptor, full []string, pane string) ([]string, bool) {
	for k, id := range full {
		root := effectiveRoot(pages[id].Body)
		embedded := k+1 < len(full)
		if embedded && str(root["pane_id"]) == pane {
			return full[k+1:], true
		}
		if leaves, _ := paneLeaves(root); leaves[pane] {
			return full[k:], true
		}
	}
	return nil, false
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

// moduleRef is a code module a class copy in a page names, of a module type.
type moduleRef struct {
	kind   string // storedModule, storedPageLayouterModule, storedElementLayouterModule or storedValidatorModule
	module string
	page   string
	path   string
}

// pageModules lists the code modules a valid page's class copies name: its page
// layouter's, each panel's element layouter's, each element instance's and each
// of its validator instances'.
func pageModules(pageID string, page map[string]any, els []mergedElement) []moduleRef {
	var out []moduleRef
	add := func(kind string, holder map[string]any, path string) {
		row, _ := holder[copyKey].(map[string]any)
		out = append(out, moduleRef{kind, str(row["code_module_id"]), pageID, path + "/" + copyKey + "/code_module_id"})
	}
	pl, _ := page["page_layouter"].(map[string]any)
	add(storedPageLayouterModule, pl, "/page_layouter")
	panels, _ := page["panels"].(map[string]any)
	for _, paneID := range slices.Sorted(maps.Keys(panels)) {
		panel, _ := panels[paneID].(map[string]any)
		el, _ := panel["elem_layouter"].(map[string]any)
		add(storedElementLayouterModule, el, "/panels/"+paneID+"/elem_layouter")
	}
	for _, e := range els {
		out = append(out, moduleRef{storedModule, str(e.copy["code_module_id"]), pageID, e.path + "/" + copyKey + "/code_module_id"})
		list, _ := e.body["validators"].([]any)
		for i, item := range list {
			m, _ := item.(map[string]any)
			row, _ := m[copyKey].(map[string]any)
			out = append(out, moduleRef{storedValidatorModule, str(row["code_module_id"]), pageID, fmt.Sprintf("%s/validators/%d/%s/code_module_id", e.path, i, copyKey)})
		}
	}
	return out
}

// moduleErrors reports class copies whose code_module_id does not name an
// active code module of the right type in kb.code_modules (OpenSpec
// wpi-code-modules, design D9; wpi-pages-own-panes, design D6).
func moduleErrors(ctx context.Context, registered Registered, refs []moduleRef) ([]Issue, error) {
	names := map[string][]string{}
	for _, r := range refs {
		if idPattern.MatchString(r.module) && !slices.Contains(names[r.kind], r.module) {
			names[r.kind] = append(names[r.kind], r.module)
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
	for _, r := range refs {
		if idPattern.MatchString(r.module) && !found[r.kind][r.module] {
			out = append(out, Issue{KindPage, r.page, r.path, fmt.Sprintf("code module %q is not an active %s module in kb.code_modules", r.module, moduleTypes[r.kind])})
		}
	}
	return out, nil
}

// copyWarnings reports, once per class and page, the attributes an element
// class copy names that its code module does not support.
func copyWarnings(pageID string, els []mergedElement) []Issue {
	var out []Issue
	done := map[string]bool{}
	for _, e := range els {
		if done[e.cls] {
			continue
		}
		done[e.cls] = true
		for _, w := range classWarnings(e.cls, e.copy) {
			out = append(out, Issue{KindPage, pageID, e.path + "/" + copyKey + w.Path, w.Message})
		}
	}
	return out
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
