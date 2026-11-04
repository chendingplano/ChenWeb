package wpi

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

// copySite is a place in a page that should hold a class copy: an element
// instance, an action instance, or a page or element layouter copy.
type copySite struct {
	kind    Kind
	nameKey string // element, action or layouter
	holder  map[string]any
	path    string
}

// copySites lists every element instance with its validator instances, action
// instance (not static data) and layouter copy of a page. It accepts pages that
// fail validation, so it can run before them.
func copySites(page map[string]any) []copySite {
	var out []copySite
	if pl, ok := page["page_layouter"].(map[string]any); ok {
		out = append(out, copySite{KindPageLayouter, "layouter", pl, "/page_layouter"})
	}
	panels, _ := page["panels"].(map[string]any)
	for _, paneID := range slices.Sorted(maps.Keys(panels)) {
		panel, _ := panels[paneID].(map[string]any)
		path := "/panels/" + paneID
		if el, ok := panel["elem_layouter"].(map[string]any); ok {
			out = append(out, copySite{KindElementLayouter, "layouter", el, path + "/elem_layouter"})
		}
		list, _ := panel["elem_list"].([]any)
		for i, e := range list {
			if m, ok := e.(map[string]any); ok {
				out = append(out, copySite{KindElement, "element", m, fmt.Sprintf("%s/elem_list/%d", path, i)})
				// The element's validator instances (OpenSpec wpi-primitive-validators, D3).
				vals, _ := m["validators"].([]any)
				for j, val := range vals {
					if vm, ok := val.(map[string]any); ok {
						out = append(out, copySite{KindValidator, "validator", vm, fmt.Sprintf("%s/elem_list/%d/validators/%d", path, i, j)})
					}
				}
			}
		}
	}
	for _, pos := range actionPositions(page) {
		if m, ok := pos.val.(map[string]any); ok && !pos.isStatic() {
			out = append(out, copySite{KindAction, "action", m, pos.path})
		}
	}
	return out
}

// fillClassCopies copies the latest active class row into every element
// instance, action instance and layouter copy of a page that has no "class"
// yet, and leaves copies already there unchanged (OpenSpec wpi-pages-own-panes,
// design D4). A class with no active row is an issue at the instance's name;
// a missing or malformed name is left to validation.
func fillClassCopies(ctx context.Context, pageID string, page map[string]any, load Loader) ([]Issue, error) {
	var missing []copySite
	names := map[Kind][]string{}
	for _, s := range copySites(page) {
		name, _ := s.holder[s.nameKey].(string)
		if _, has := s.holder[copyKey]; has || !idPattern.MatchString(name) {
			continue
		}
		missing = append(missing, s)
		if !slices.Contains(names[s.kind], name) {
			names[s.kind] = append(names[s.kind], name)
		}
	}
	rows := map[Kind]map[string]Descriptor{}
	for _, kind := range slices.Sorted(maps.Keys(names)) {
		var err error
		if rows[kind], err = load(ctx, kind, names[kind]); err != nil {
			return nil, err
		}
	}
	var issues []Issue
	for _, s := range missing {
		name := s.holder[s.nameKey].(string)
		row, ok := rows[s.kind][name]
		if !ok {
			issues = append(issues, Issue{KindPage, pageID, s.path + "/" + s.nameKey, fmt.Sprintf("%s class %q has no active row to copy", s.kind, name)})
			continue
		}
		// Each site gets its own copy, decoded afresh, so no two share a map.
		copied, err := decodeBody(row.Raw)
		if err != nil {
			return nil, fmt.Errorf("%s class %q: %w", s.kind, name, err)
		}
		s.holder[copyKey] = copied
	}
	return issues, nil
}
