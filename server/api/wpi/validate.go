package wpi

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
)

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,127}$`)
	columnPattern = segmentPattern
	tablePattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)
	sizePattern   = regexp.MustCompile(`^(auto|elastic|[0-9]+(\.[0-9]+)?(px|rem|em|%))$`)
)

var elementSizeAttrs = []string{"width", "height", "min_width", "max_width"}

// elementAttrs lists the attributes each element type accepts besides
// descriptor_id, schema_version, element_type and the size attributes.
var elementAttrs = map[string]struct{ required, optional []string }{
	"text":   {nil, []string{"text", "data_binding"}},
	"input":  {[]string{"label", "data_binding", "binding_mode"}, []string{"input_type"}},
	"select": {[]string{"label", "data_binding", "binding_mode", "options"}, nil},
	"button": {[]string{"label", "on_click"}, nil},
	"list":   {[]string{"source", "presentation", "columns"}, []string{"on_row_click"}},
	"menu":   {[]string{"items"}, []string{"initial_item"}},
}

type validator struct {
	kind   Kind
	id     string
	issues []Issue
}

func (v *validator) errf(path, format string, a ...any) {
	v.issues = append(v.issues, Issue{Kind: v.kind, DescriptorID: v.id, Path: path, Message: fmt.Sprintf(format, a...)})
}

// ValidateDescriptor checks one stored descriptor's structure and that every
// type it names is supported. It does not follow references to other
// descriptors; BuildBundle does that.
func ValidateDescriptor(kind Kind, id string, body map[string]any) []Issue {
	v := &validator{kind: kind, id: id}
	if !idPattern.MatchString(id) {
		v.errf("", "descriptor ID %q must match %s", id, idPattern)
	}
	if got, _ := body["descriptor_id"].(string); got != id {
		v.errf("/descriptor_id", "descriptor_id %q does not match %q", got, id)
	}
	if sv, _ := body["schema_version"].(string); sv != SchemaVersion {
		v.errf("/schema_version", "schema_version %q is not supported by this interpreter (supports %q)", sv, SchemaVersion)
	}
	switch kind {
	case KindApp:
		v.app(body)
	case KindPage:
		v.page(body)
	case KindElement:
		v.element(body, "", true)
	case KindAction:
		v.action(body, "", true)
	default:
		v.errf("", "unknown descriptor kind %q", kind)
	}
	return v.issues
}

// keys reports missing required attributes and attributes not in either list.
func (v *validator) keys(obj map[string]any, path string, required []string, optional ...string) {
	allowed := map[string]bool{}
	for _, k := range required {
		allowed[k] = true
		if _, ok := obj[k]; !ok {
			v.errf(path, "missing attribute %q", k)
		}
	}
	for _, k := range optional {
		allowed[k] = true
	}
	for _, k := range slices.Sorted(maps.Keys(obj)) {
		if !allowed[k] {
			v.errf(path+"/"+k, "unknown attribute %q", k)
		}
	}
}

func (v *validator) object(val any, path string) (map[string]any, bool) {
	m, ok := val.(map[string]any)
	if !ok {
		v.errf(path, "must be an object")
	}
	return m, ok
}

func (v *validator) array(val any, path string, nonEmpty bool) ([]any, bool) {
	a, ok := val.([]any)
	if !ok {
		v.errf(path, "must be an array")
		return nil, false
	}
	if nonEmpty && len(a) == 0 {
		v.errf(path, "must not be empty")
		return nil, false
	}
	return a, true
}

// str checks an optional string attribute; present values must be non-empty.
func (v *validator) str(obj map[string]any, key, path string) (string, bool) {
	val, present := obj[key]
	if !present {
		return "", false
	}
	s, ok := val.(string)
	if !ok || s == "" {
		v.errf(path+"/"+key, "must be a non-empty string")
		return "", false
	}
	return s, true
}

func (v *validator) matching(obj map[string]any, key, path string, re *regexp.Regexp) (string, bool) {
	s, ok := v.str(obj, key, path)
	if ok && !re.MatchString(s) {
		v.errf(path+"/"+key, "%q must match %s", s, re)
		return "", false
	}
	return s, ok
}

func (v *validator) oneOf(obj map[string]any, key, path string, allowed []string) (string, bool) {
	s, ok := v.str(obj, key, path)
	if ok && !slices.Contains(allowed, s) {
		v.errf(path+"/"+key, "%q is not supported (supported: %v)", s, allowed)
		return "", false
	}
	return s, ok
}

func (v *validator) boolean(obj map[string]any, key, path string) {
	if val, present := obj[key]; present {
		if _, ok := val.(bool); !ok {
			v.errf(path+"/"+key, "must be true or false")
		}
	}
}

func (v *validator) size(obj map[string]any, key, path string) {
	v.matching(obj, key, path, sizePattern)
}

func (v *validator) positiveInt(obj map[string]any, key, path string) {
	val, present := obj[key]
	if !present {
		return
	}
	if n, ok := asInt(val); !ok || n <= 0 {
		v.errf(path+"/"+key, "must be a positive integer")
	}
}

func asInt(val any) (int64, bool) {
	switch n := val.(type) {
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case float64:
		return int64(n), n == float64(int64(n))
	}
	return 0, false
}

func (v *validator) values(obj map[string]any, key, path string, roots ...string) {
	val, present := obj[key]
	if !present {
		return
	}
	m, ok := v.object(val, path+"/"+key)
	if !ok {
		return
	}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if err := checkValue(m[k], roots...); err != nil {
			v.errf(path+"/"+key+"/"+k, "%v", err)
		}
	}
}

func (v *validator) app(b map[string]any) {
	v.keys(b, "", []string{"descriptor_id", "schema_version", "layout_template", "panes"})
	tmpl, _ := v.oneOf(b, "layout_template", "", slices.Sorted(maps.Keys(templatePanes)))
	panes, ok := b["panes"]
	if !ok {
		return
	}
	m, ok := v.object(panes, "/panes")
	if !ok {
		return
	}
	for _, paneID := range slices.Sorted(maps.Keys(m)) {
		path := "/panes/" + paneID
		if tmpl != "" && !slices.Contains(templatePanes[tmpl], paneID) {
			v.errf(path, "pane %q is not part of layout template %q", paneID, tmpl)
		}
		p, ok := v.object(m[paneID], path)
		if !ok {
			continue
		}
		v.keys(p, path, nil, "width", "height", "min_width", "max_width", "min_height", "max_height", "resizable", "scroll", "page")
		for _, k := range []string{"width", "height", "min_width", "max_width", "min_height", "max_height"} {
			v.size(p, k, path)
		}
		v.boolean(p, "resizable", path)
		v.oneOf(p, "scroll", path, []string{"auto", "none"})
		v.matching(p, "page", path, idPattern)
	}
}

func (v *validator) page(b map[string]any) {
	v.keys(b, "", []string{"descriptor_id", "schema_version", "layouter", "elements"}, "input", "save_action", "refresh")
	v.oneOf(b, "layouter", "", layouters)
	if in, ok := b["input"]; ok {
		v.input(in, "/input")
	}
	v.matching(b, "save_action", "", idPattern)
	if r, ok := b["refresh"]; ok {
		if m, ok := v.object(r, "/refresh"); ok {
			v.keys(m, "/refresh", nil, "on_show")
			v.boolean(m, "on_show", "/refresh")
		}
	}
	els, ok := b["elements"]
	if !ok {
		return
	}
	list, ok := v.array(els, "/elements", false)
	if !ok {
		return
	}
	inlineIDs := map[string]bool{}
	for i, e := range list {
		path := "/elements/" + strconv.Itoa(i)
		m, ok := v.object(e, path)
		if !ok {
			continue
		}
		if _, isRef := m["element"]; isRef {
			v.elementRef(m, path)
			continue
		}
		v.element(m, path, false)
		if id, _ := m["descriptor_id"].(string); id != "" {
			if inlineIDs[id] {
				v.errf(path+"/descriptor_id", "inline element ID %q is used twice in this page", id)
			}
			inlineIDs[id] = true
		}
	}
}

// elementRef checks a page entry that places a stored element. Override
// values are checked against the element's type when the bundle is built.
func (v *validator) elementRef(m map[string]any, path string) {
	v.matching(m, "element", path, idPattern)
	allowed := map[string]bool{}
	for _, a := range elementAttrs {
		for _, k := range append(slices.Clone(a.required), a.optional...) {
			allowed[k] = true
		}
	}
	for _, k := range elementSizeAttrs {
		allowed[k] = true
	}
	for _, k := range slices.Sorted(maps.Keys(m)) {
		if k != "element" && !allowed[k] {
			v.errf(path+"/"+k, "attribute %q cannot be overridden", k)
		}
	}
}

func (v *validator) element(e map[string]any, path string, stored bool) {
	typ, _ := e["element_type"].(string)
	attrs, known := elementAttrs[typ]
	if !known {
		if typ == "" {
			v.errf(path, "missing attribute %q", "element_type")
		} else {
			v.errf(path+"/element_type", "element type %q is not supported", typ)
		}
		return
	}
	required := append([]string{"descriptor_id", "element_type"}, attrs.required...)
	optional := append(slices.Clone(elementSizeAttrs), attrs.optional...)
	if stored {
		required = append(required, "schema_version")
	} else {
		v.matching(e, "descriptor_id", path, idPattern)
	}
	v.keys(e, path, required, optional...)
	for _, k := range elementSizeAttrs {
		v.size(e, k, path)
	}
	switch typ {
	case "text":
		_, hasText := e["text"]
		_, hasBinding := e["data_binding"]
		if hasText == hasBinding {
			v.errf(path, "a text element needs exactly one of %q or %q", "text", "data_binding")
		}
		if hasText {
			if _, ok := e["text"].(string); !ok {
				v.errf(path+"/text", "must be a string")
			}
		}
		v.binding(e, path)
	case "input", "select":
		v.str(e, "label", path)
		v.binding(e, path)
		v.oneOf(e, "binding_mode", path, []string{"read-only", "read-write"})
		v.oneOf(e, "input_type", path, []string{"text", "number", "email"})
		if typ == "select" {
			v.options(e, path)
		}
	case "button":
		v.str(e, "label", path)
		if t, ok := e["on_click"]; ok {
			v.trigger(t, path+"/on_click", false)
		}
	case "list":
		v.listElement(e, path)
	case "menu":
		v.menu(e, path)
	}
}

func (v *validator) binding(e map[string]any, path string) {
	s, ok := v.str(e, "data_binding", path)
	if !ok {
		return
	}
	ref, isRef, err := parseRef(s)
	switch {
	case err != nil:
		v.errf(path+"/data_binding", "%v", err)
	case !isRef || ref.Root != RootInputDoc:
		v.errf(path+"/data_binding", "must be a $input_doc path")
	}
}

func (v *validator) options(e map[string]any, path string) {
	list, ok := v.array(e["options"], path+"/options", true)
	if !ok {
		return
	}
	for i, o := range list {
		p := path + "/options/" + strconv.Itoa(i)
		m, ok := v.object(o, p)
		if !ok {
			continue
		}
		v.keys(m, p, []string{"value", "label"})
		v.str(m, "label", p)
		switch m["value"].(type) {
		case string, json.Number, float64:
		default:
			v.errf(p+"/value", "must be a string or number")
		}
	}
}

func (v *validator) listElement(e map[string]any, path string) {
	if src, ok := e["source"]; ok {
		if m, ok := v.object(src, path+"/source"); ok {
			v.keys(m, path+"/source", []string{"action"}, "args")
			v.matching(m, "action", path+"/source", idPattern)
			v.values(m, "args", path+"/source", RootInputDoc)
		}
	}
	v.oneOf(e, "presentation", path, []string{"rows"})
	if cols, ok := e["columns"]; ok {
		if list, ok := v.array(cols, path+"/columns", true); ok {
			for i, c := range list {
				p := path + "/columns/" + strconv.Itoa(i)
				m, ok := v.object(c, p)
				if !ok {
					continue
				}
				v.keys(m, p, []string{"field", "label"}, "width")
				v.matching(m, "field", p, segmentPattern)
				v.str(m, "label", p)
				v.size(m, "width", p)
			}
		}
	}
	if t, ok := e["on_row_click"]; ok {
		v.trigger(t, path+"/on_row_click", true)
	}
}

func (v *validator) menu(e map[string]any, path string) {
	items, ok := e["items"]
	if !ok {
		return
	}
	leaves := map[string]bool{}
	seen := map[string]bool{}
	v.menuItems(items, path+"/items", seen, leaves)
	if initial, ok := v.str(e, "initial_item", path); ok && !leaves[initial] {
		v.errf(path+"/initial_item", "initial_item %q is not a menu item that opens a page", initial)
	}
}

func (v *validator) menuItems(val any, path string, seen, leaves map[string]bool) {
	list, ok := v.array(val, path, true)
	if !ok {
		return
	}
	for i, it := range list {
		p := path + "/" + strconv.Itoa(i)
		m, ok := v.object(it, p)
		if !ok {
			continue
		}
		v.keys(m, p, []string{"item_id", "label"}, "on_click", "children")
		id, _ := v.matching(m, "item_id", p, idPattern)
		if id != "" {
			if seen[id] {
				v.errf(p+"/item_id", "menu item ID %q is used twice", id)
			}
			seen[id] = true
		}
		v.str(m, "label", p)
		click, hasClick := m["on_click"]
		children, hasChildren := m["children"]
		switch {
		case hasClick == hasChildren:
			v.errf(p, "a menu item needs exactly one of %q or %q", "on_click", "children")
		case hasClick:
			v.trigger(click, p+"/on_click", false)
			if id != "" {
				leaves[id] = true
			}
		default:
			v.menuItems(children, p+"/children", seen, leaves)
		}
	}
}

// trigger checks an event's action: a stored action by ID or an inline action.
// Its args may use $input_doc, and $record when allowRecord is set.
func (v *validator) trigger(val any, path string, allowRecord bool) {
	t, ok := v.object(val, path)
	if !ok {
		return
	}
	roots := []string{RootInputDoc}
	if allowRecord {
		roots = append(roots, RootRecord)
	}
	if _, byID := t["action"]; byID {
		v.keys(t, path, []string{"action"}, "args")
		v.matching(t, "action", path, idPattern)
	} else {
		v.action(t, path, false, "args")
	}
	v.values(t, "args", path, roots...)
}

func (v *validator) input(val any, path string, roots ...string) {
	m, ok := v.object(val, path)
	if !ok || len(m) == 0 {
		return
	}
	v.keys(m, path, []string{"action"}, "args")
	v.matching(m, "action", path, idPattern)
	v.values(m, "args", path, roots...)
}

// action checks a stored action (stored=true) or an inline one inside a
// trigger; extra lists attributes the surrounding trigger adds.
func (v *validator) action(a map[string]any, path string, stored bool, extra ...string) {
	typ, _ := a["action_type"].(string)
	if !slices.Contains(actionTypes, typ) {
		if typ == "" {
			v.errf(path, "missing attribute %q", "action_type")
		} else {
			v.errf(path+"/action_type", "action type %q is not supported", typ)
		}
		return
	}
	required := []string{"action_type"}
	if stored {
		required = append(required, "descriptor_id", "schema_version")
	}
	switch typ {
	case "load":
		v.keys(a, path, append(required, "table", "result", "fields"), append(extra, "where", "order_by", "limit")...)
		v.matching(a, "table", path, tablePattern)
		v.oneOf(a, "result", path, []string{"record", "list"})
		v.fieldMaps(a, path, false)
		if w, ok := a["where"]; ok {
			if m, ok := v.object(w, path+"/where"); ok {
				for _, col := range slices.Sorted(maps.Keys(m)) {
					if !columnPattern.MatchString(col) {
						v.errf(path+"/where/"+col, "invalid column name %q", col)
					}
				}
			}
			v.values(a, "where", path, RootArgs)
		}
		v.orderBy(a, path)
		v.positiveInt(a, "limit", path)
	case "save":
		v.keys(a, path, append(required, "table", "mode", "key", "fields"), extra...)
		v.matching(a, "table", path, tablePattern)
		v.oneOf(a, "mode", path, []string{"insert", "update", "upsert"})
		v.matching(a, "key", path, columnPattern)
		v.fieldMaps(a, path, true)
	case "open_page":
		v.keys(a, path, append(required, "page", "pane"), append(extra, "input")...)
		v.matching(a, "page", path, idPattern)
		v.str(a, "pane", path)
		if in, ok := a["input"]; ok {
			v.input(in, path+"/input", RootArgs)
		}
	case "close_page":
		v.keys(a, path, required, extra...)
	}
}

func (v *validator) fieldMaps(a map[string]any, path string, save bool) {
	fs, ok := a["fields"]
	if !ok {
		return
	}
	list, ok := v.array(fs, path+"/fields", true)
	if !ok {
		return
	}
	for i, f := range list {
		p := path + "/fields/" + strconv.Itoa(i)
		m, ok := v.object(f, p)
		if !ok {
			continue
		}
		if save {
			v.keys(m, p, []string{"attribute", "column"}, "required", "rules")
			v.boolean(m, "required", p)
			v.rules(m, p)
		} else {
			v.keys(m, p, []string{"column", "attribute"})
		}
		v.matching(m, "attribute", p, segmentPattern)
		v.matching(m, "column", p, columnPattern)
	}
}

func (v *validator) rules(m map[string]any, path string) {
	r, ok := m["rules"]
	if !ok {
		return
	}
	rm, ok := v.object(r, path+"/rules")
	if !ok {
		return
	}
	p := path + "/rules"
	v.keys(rm, p, nil, "max_length", "pattern", "format")
	v.positiveInt(rm, "max_length", p)
	if pat, ok := v.str(rm, "pattern", p); ok {
		if _, err := regexp.Compile(pat); err != nil {
			v.errf(p+"/pattern", "invalid pattern: %v", err)
		}
	}
	v.oneOf(rm, "format", p, []string{"email"})
}

func (v *validator) orderBy(a map[string]any, path string) {
	ob, ok := a["order_by"]
	if !ok {
		return
	}
	list, ok := v.array(ob, path+"/order_by", true)
	if !ok {
		return
	}
	for i, o := range list {
		p := path + "/order_by/" + strconv.Itoa(i)
		m, ok := v.object(o, p)
		if !ok {
			continue
		}
		v.keys(m, p, []string{"column"}, "direction")
		v.matching(m, "column", p, columnPattern)
		v.oneOf(m, "direction", p, []string{"asc", "desc"})
	}
}
