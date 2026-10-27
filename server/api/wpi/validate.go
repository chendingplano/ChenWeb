package wpi

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,127}$`)
	columnPattern = segmentPattern
	tablePattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)
	sizePattern   = regexp.MustCompile(`^(auto|elastic|[0-9]+(\.[0-9]+)?(px|rem|em|%))$`)
	// A page's size is set, not computed by a layouter, so it cannot be elastic.
	pageSizePattern = regexp.MustCompile(`^(auto|[0-9]+(\.[0-9]+)?(px|rem|em|%))$`)
	localePattern   = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})*$`)
)

var elementSizeAttrs = []string{"width", "height", "min_width", "max_width"}

var bindingModes = []string{"read-only", "read-write"}

// codeAttrs lists the instance attributes a class's code defines: required
// ones the code cannot work without, and optional ones.
type codeAttrs struct{ required, optional []string }

func (a codeAttrs) defines(attr string) bool {
	return slices.Contains(a.required, attr) || slices.Contains(a.optional, attr)
}

// elementAttrs is keyed by element class ID, which is also the name of the
// frontend code that draws the class (devdoc 2026102601 §3.5.1). Every element
// class also defines the size attributes.
var elementAttrs = map[string]codeAttrs{
	"text":   {nil, []string{"text", "data_binding"}},
	"input":  {[]string{"label", "data_binding", "binding_mode"}, []string{"input_type"}},
	"select": {[]string{"label", "data_binding", "binding_mode", "options"}, nil},
	"button": {[]string{"label", "on_click"}, nil},
	"list":   {[]string{"source", "presentation", "columns"}, []string{"on_row_click"}},
	"menu":   {[]string{"items"}, []string{"initial_item"}},
}

// actionAttrs is keyed by action class ID, which is also the name of the code
// that performs the action (devdoc §5). Every action instance may also set args.
var actionAttrs = map[string]codeAttrs{
	"load":       {[]string{"table", "result", "fields"}, []string{"where", "order_by", "limit"}},
	"save":       {[]string{"table", "mode", "key", "fields"}, nil},
	"open-page":  {[]string{"page", "pane"}, []string{"input"}},
	"close-page": {},
	"save-page":  {},
	"query":      {[]string{"sql_stmt_id", "result"}, []string{"params"}},
	"load-file":  {[]string{"file_id"}, nil},
}

// classAttrs are shared by every instance of a class and set only in its row.
var classAttrs = []string{"descriptor_id", "schema_version", "description"}

// actionHolding attributes hold action instances. A class row cannot give them
// defaults, so every action instance is written in a page (design D5).
var actionHolding = []string{"on_click", "on_row_click", "source", "items", "input"}

var (
	triggerClasses = []string{"save", "open-page", "close-page", "save-page"}
	inputClasses   = []string{"load", "query", "load-file"}
	sourceClasses  = []string{"load", "query"}
	saveClass      = []string{"save"}
)

// classCode returns the instance attributes the code of a class defines.
func classCode(kind Kind, cls string) (codeAttrs, bool) {
	if kind == KindAction {
		a, ok := actionAttrs[cls]
		return a, ok
	}
	a, ok := elementAttrs[cls]
	a.optional = append(slices.Clone(a.optional), elementSizeAttrs...)
	return a, ok
}

type validator struct {
	kind   Kind
	id     string
	issues []Issue
	// classes, when set, are the class rows instances are checked against.
	classes map[Kind]map[string]Descriptor
}

func (v *validator) errf(path, format string, a ...any) {
	v.issues = append(v.issues, Issue{Kind: v.kind, DescriptorID: v.id, Path: path, Message: fmt.Sprintf(format, a...)})
}

// ValidateDescriptor checks one stored descriptor's structure and that every
// class it names has code. It checks a page's instances only as far as that is
// possible without class rows; BuildBundle checks them against the rows and
// follows references to other descriptors.
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
	case KindElement, KindAction:
		v.classRow(body)
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
	v.keys(b, "", []string{"descriptor_id", "schema_version", "layouter", "elements"}, "input", "save_action", "refresh",
		"page_name", "description", "page_alignment", "width", "height", "min_width", "max_width", "binding_mode")
	v.oneOf(b, "layouter", "", layouters)
	v.text(b, "page_name", "")
	v.text(b, "description", "")
	v.oneOf(b, "page_alignment", "", []string{"left", "center", "right"})
	for _, k := range elementSizeAttrs {
		v.matching(b, k, "", pageSizePattern)
	}
	v.oneOf(b, "binding_mode", "", bindingModes)
	if r, ok := b["refresh"]; ok {
		if m, ok := v.object(r, "/refresh"); ok {
			v.keys(m, "/refresh", nil, "on_show")
			v.boolean(m, "on_show", "/refresh")
		}
	}
	if els, ok := b["elements"]; ok {
		if list, ok := v.array(els, "/elements", false); ok {
			for i, e := range list {
				path := "/elements/" + strconv.Itoa(i)
				if m, ok := v.object(e, path); ok {
					v.elementInstance(m, path)
				}
			}
		}
	}
	for _, pos := range actionPositions(b) {
		v.actionInstance(pos)
	}
}

// classRow checks an element or action class row (design D2): besides the
// class attributes, it holds instance attributes its code defines, each with a
// default or null. Every attribute the code requires must be there, and
// attributes that hold actions cannot have defaults.
func (v *validator) classRow(b map[string]any) {
	code, known := classCode(v.kind, v.id)
	if !known {
		v.errf("", "%s class %q has no code in this interpreter", v.kind, v.id)
		return
	}
	v.text(b, "description", "")
	defaults := map[string]any{}
	for _, k := range slices.Sorted(maps.Keys(b)) {
		switch {
		case slices.Contains(classAttrs, k):
		case !code.defines(k):
			v.errf("/"+k, "unknown attribute %q", k)
		case b[k] == nil:
		case slices.Contains(actionHolding, k):
			v.errf("/"+k, "attribute %q holds actions, so it cannot have a default; use null or leave it out", k)
		default:
			defaults[k] = b[k]
		}
	}
	for _, k := range code.required {
		if _, ok := b[k]; !ok {
			v.errf("", "the class must give %q a default or null, because its code requires it", k)
		}
	}
	if v.kind == KindElement {
		v.element(v.id, defaults, "")
	} else {
		v.action(v.id, defaults, "")
	}
}

// elementInstance checks a page entry {"element": class, ...} as far as it can
// without the class row; pageInstances checks it against the row.
func (v *validator) elementInstance(m map[string]any, path string) {
	cls, ok := v.str(m, "element", path)
	if !ok {
		if _, present := m["element"]; !present {
			v.errf(path, "missing attribute %q", "element")
		}
		return
	}
	if _, known := elementAttrs[cls]; !known {
		v.errf(path+"/element", "element class %q has no code in this interpreter", cls)
		return
	}
	v.noClassAttrs(m, path)
	v.element(cls, instanceAttrs(m, "element"), path)
}

func (v *validator) noClassAttrs(m map[string]any, path string) {
	for _, k := range classAttrs {
		if _, ok := m[k]; ok {
			v.errf(path+"/"+k, "%q is a class attribute; an instance cannot set it", k)
		}
	}
}

// instanceAttrs returns an instance's own attributes: everything but its class
// name (classKey) and class attributes.
func instanceAttrs(m map[string]any, classKey string) map[string]any {
	out := map[string]any{}
	for k, val := range m {
		if k != classKey && !slices.Contains(classAttrs, k) {
			out[k] = val
		}
	}
	return out
}

// element checks the attributes an element instance or class row sets: each is
// defined by the class's code and has a valid value. Whether required ones are
// there is checked on the merged instance (elementComplete). Action instances
// inside are checked through actionPositions.
func (v *validator) element(cls string, e map[string]any, path string) {
	code, _ := classCode(KindElement, cls)
	v.keys(e, path, nil, append(slices.Clone(code.required), code.optional...)...)
	for _, k := range elementSizeAttrs {
		v.size(e, k, path)
	}
	switch cls {
	case "text":
		if t, ok := e["text"]; ok {
			if _, ok := t.(string); !ok {
				v.errf(path+"/text", "must be a string")
			}
		}
		v.binding(e, path)
	case "input", "select":
		v.text(e, "label", path)
		v.binding(e, path)
		v.oneOf(e, "binding_mode", path, bindingModes)
		v.oneOf(e, "input_type", path, []string{"text", "number", "email"})
		if _, ok := e["options"]; ok {
			v.options(e, path)
		}
	case "button":
		v.text(e, "label", path)
	case "list":
		v.listColumns(e, path)
		v.oneOf(e, "presentation", path, []string{"rows"})
	case "menu":
		if items, ok := e["items"]; ok {
			v.menuItems(items, path+"/items", map[string]bool{})
		}
		v.str(e, "initial_item", path)
	}
}

// elementComplete checks a merged element instance: every attribute its code
// requires is there, and the rules that span attributes hold.
func (v *validator) elementComplete(cls string, merged map[string]any, path string) {
	code, _ := classCode(KindElement, cls)
	v.required(merged, path, code.required)
	switch cls {
	case "text":
		_, hasText := merged["text"]
		_, hasBinding := merged["data_binding"]
		if hasText == hasBinding {
			v.errf(path, "a text element needs exactly one of %q or %q", "text", "data_binding")
		}
	case "menu":
		if initial, ok := merged["initial_item"].(string); ok && !menuLeaves(merged["items"])[initial] {
			v.errf(path+"/initial_item", "initial_item %q is not a menu item that opens a page", initial)
		}
	}
}

func (v *validator) required(obj map[string]any, path string, keys []string) {
	for _, k := range keys {
		if _, ok := obj[k]; !ok {
			v.errf(path, "missing attribute %q", k)
		}
	}
}

// text checks a text attribute: a non-empty string, or non-empty strings keyed
// by language code that include "en" (design D9).
func (v *validator) text(obj map[string]any, key, path string) {
	val, present := obj[key]
	if !present {
		return
	}
	p := path + "/" + key
	switch t := val.(type) {
	case string:
		if t == "" {
			v.errf(p, "must be a non-empty string or an object of texts by language")
		}
	case map[string]any:
		for _, lang := range slices.Sorted(maps.Keys(t)) {
			if !localePattern.MatchString(lang) {
				v.errf(p+"/"+lang, "%q is not a language code", lang)
			}
			if s, ok := t[lang].(string); !ok || s == "" {
				v.errf(p+"/"+lang, "must be a non-empty string")
			}
		}
		if _, ok := t["en"]; !ok {
			v.errf(p, "texts by language need an %q entry", "en")
		}
	default:
		v.errf(p, "must be a non-empty string or an object of texts by language")
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

func (v *validator) listColumns(e map[string]any, path string) {
	cols, ok := e["columns"]
	if !ok {
		return
	}
	list, ok := v.array(cols, path+"/columns", true)
	if !ok {
		return
	}
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

func (v *validator) menuItems(val any, path string, seen map[string]bool) {
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
		if id, ok := v.matching(m, "item_id", p, idPattern); ok {
			if seen[id] {
				v.errf(p+"/item_id", "menu item ID %q is used twice", id)
			}
			seen[id] = true
		}
		v.str(m, "label", p)
		_, hasClick := m["on_click"]
		children, hasChildren := m["children"]
		switch {
		case hasClick == hasChildren:
			v.errf(p, "a menu item needs exactly one of %q or %q", "on_click", "children")
		case hasChildren:
			v.menuItems(children, p+"/children", seen)
		}
	}
}

// menuLeaves returns the IDs of menu items that open a page.
func menuLeaves(items any) map[string]bool {
	out := map[string]bool{}
	list, _ := items.([]any)
	for _, it := range list {
		m, _ := it.(map[string]any)
		if _, ok := m["on_click"]; ok {
			out[str(m["item_id"])] = true
		}
		maps.Copy(out, menuLeaves(m["children"]))
	}
	return out
}

// actionPos is a place in a page that holds an action instance.
type actionPos struct {
	val      any
	path     string
	allowed  []string // action classes allowed here
	roots    []string // reference roots its args may use
	staticOK bool     // an object without "action" is static data: the input doc itself
}

// isStatic reports whether the value at pos is static data rather than an
// action instance (devdoc 2026102601 §3.2.2).
func (pos actionPos) isStatic() bool {
	m, _ := pos.val.(map[string]any)
	_, hasAction := m["action"]
	return pos.staticOK && m != nil && !hasAction
}

// actionPositions lists every place in a page that holds an action instance
// (design D5), including open-page inputs nested in them. It accepts pages
// that fail validation, so callers can use it on any stored page.
func actionPositions(page map[string]any) []actionPos {
	var out []actionPos
	if in, ok := page["input"]; ok {
		out = append(out, actionPos{in, "/input", inputClasses, nil, true})
	}
	if s, ok := page["save_action"]; ok {
		out = append(out, actionPos{s, "/save_action", saveClass, nil, false})
	}
	els, _ := page["elements"].([]any)
	for i, e := range els {
		m, _ := e.(map[string]any)
		code := elementAttrs[str(m["element"])]
		path := "/elements/" + strconv.Itoa(i)
		add := func(attr string, allowed, roots []string) {
			if val, ok := m[attr]; ok && code.defines(attr) {
				out = append(out, actionPos{val, path + "/" + attr, allowed, roots, false})
			}
		}
		add("on_click", triggerClasses, []string{RootInputDoc})
		add("source", sourceClasses, []string{RootInputDoc})
		add("on_row_click", triggerClasses, []string{RootInputDoc, RootRecord})
		if code.defines("items") {
			out = append(out, itemPositions(m["items"], path+"/items")...)
		}
	}
	for i := 0; i < len(out); i++ {
		m, _ := out[i].val.(map[string]any)
		if in, ok := m["input"]; ok && str(m["action"]) == "open-page" {
			out = append(out, actionPos{in, out[i].path + "/input", inputClasses, []string{RootArgs}, true})
		}
	}
	return out
}

func itemPositions(items any, path string) []actionPos {
	var out []actionPos
	list, _ := items.([]any)
	for i, it := range list {
		m, _ := it.(map[string]any)
		p := path + "/" + strconv.Itoa(i)
		if t, ok := m["on_click"]; ok {
			out = append(out, actionPos{t, p + "/on_click", triggerClasses, []string{RootInputDoc}, false})
		}
		out = append(out, itemPositions(m["children"], p+"/children")...)
	}
	return out
}

// actionInstance checks an action instance as far as it can without the class
// row: its class has code and is allowed here, and the attributes it sets are
// defined by that code and valid.
func (v *validator) actionInstance(pos actionPos) {
	m, ok := v.object(pos.val, pos.path)
	if !ok || pos.isStatic() {
		return
	}
	cls, ok := v.str(m, "action", pos.path)
	if !ok {
		if _, present := m["action"]; !present {
			v.errf(pos.path, "missing attribute %q", "action")
		}
		return
	}
	if _, known := actionAttrs[cls]; !known {
		v.errf(pos.path+"/action", "action class %q has no code in this interpreter", cls)
		return
	}
	if !slices.Contains(pos.allowed, cls) {
		v.errf(pos.path+"/action", "a %s action is not allowed here (allowed: %s)", cls, strings.Join(pos.allowed, ", "))
		return
	}
	v.noClassAttrs(m, pos.path)
	v.action(cls, instanceAttrs(m, "action"), pos.path, "args")
	v.values(m, "args", pos.path, pos.roots...)
}

// action checks the attributes an action instance or class row sets; extra
// lists attributes allowed besides those its code defines.
func (v *validator) action(cls string, a map[string]any, path string, extra ...string) {
	code := actionAttrs[cls]
	v.keys(a, path, nil, append(append(slices.Clone(code.required), code.optional...), extra...)...)
	switch cls {
	case "load":
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
		v.matching(a, "table", path, tablePattern)
		v.oneOf(a, "mode", path, []string{"insert", "update", "upsert"})
		v.matching(a, "key", path, columnPattern)
		v.fieldMaps(a, path, true)
	case "open-page":
		v.matching(a, "page", path, idPattern)
		v.str(a, "pane", path)
	case "query":
		v.matching(a, "sql_stmt_id", path, idPattern)
		v.oneOf(a, "result", path, []string{"record", "list"})
		if p, ok := a["params"]; ok {
			if list, ok := v.array(p, path+"/params", false); ok {
				for i, val := range list {
					if err := checkValue(val, RootArgs); err != nil {
						v.errf(path+"/params/"+strconv.Itoa(i), "%v", err)
					}
				}
			}
		}
	case "load-file":
		v.matching(a, "file_id", path, idPattern)
	}
}

// mergeInstance overlays an instance's own attributes on its class row's
// non-null defaults (devdoc 2026102601 §3.5.1).
func mergeInstance(row, own map[string]any) map[string]any {
	out := map[string]any{}
	for k, val := range row {
		if val != nil && !slices.Contains(classAttrs, k) {
			out[k] = val
		}
	}
	maps.Copy(out, own)
	return out
}

// againstClass checks an instance's own attributes against its class row and
// returns the merged instance. Every attribute the row sets to null must be
// set by the instance.
func (v *validator) againstClass(kind Kind, cls string, own map[string]any, path string) (map[string]any, bool) {
	row, ok := v.classes[kind][cls]
	if !ok {
		key := "element"
		if kind == KindAction {
			key = "action"
		}
		v.errf(path+"/"+key, "%s class %q does not exist", kind, cls)
		return nil, false
	}
	for _, k := range slices.Sorted(maps.Keys(row.Body)) {
		if _, set := own[k]; !set && row.Body[k] == nil {
			v.errf(path, "attribute %q has no default in %s class %q, so this instance must set it", k, kind, cls)
		}
	}
	return mergeInstance(row.Body, own), true
}

// actionComplete checks an action instance against its class row and returns
// the merged instance, or nil for static data or an instance whose class
// problems actionInstance reports.
func (v *validator) actionComplete(pos actionPos) map[string]any {
	m, _ := pos.val.(map[string]any)
	cls := str(m["action"])
	if _, known := actionAttrs[cls]; !known || !slices.Contains(pos.allowed, cls) {
		return nil
	}
	merged, ok := v.againstClass(KindAction, cls, instanceAttrs(m, "action"), pos.path)
	if !ok {
		return nil
	}
	v.required(merged, pos.path, actionAttrs[cls].required)
	return merged
}

// withPageDefaults gives an element instance that does not set binding_mode
// the page's, when its class defines one, so the order is instance, page,
// class (devdoc 2026102601 §3.2.1, design D4).
func withPageDefaults(page map[string]any, cls string, own map[string]any) map[string]any {
	mode, set := page["binding_mode"]
	if _, ownMode := own["binding_mode"]; set && !ownMode && elementAttrs[cls].defines("binding_mode") {
		own["binding_mode"] = mode
	}
	return own
}

// mergedElement is an element instance merged with its class.
type mergedElement struct {
	cls, path string
	body      map[string]any
}

// mergedAction is an action instance merged with its class.
type mergedAction struct {
	page, path, cls string
	body            map[string]any
}

// pageInstances checks a page's instances against their class rows (design
// D3, second stage) and returns them merged with their classes.
func (v *validator) pageInstances(page map[string]any) ([]mergedElement, []mergedAction) {
	var els []mergedElement
	list, _ := page["elements"].([]any)
	for i, e := range list {
		m, _ := e.(map[string]any)
		cls, path := str(m["element"]), "/elements/"+strconv.Itoa(i)
		if merged, ok := v.againstClass(KindElement, cls, withPageDefaults(page, cls, instanceAttrs(m, "element")), path); ok {
			v.elementComplete(cls, merged, path)
			els = append(els, mergedElement{cls, path, merged})
		}
	}
	var acts []mergedAction
	for _, pos := range actionPositions(page) {
		if merged := v.actionComplete(pos); merged != nil {
			m, _ := pos.val.(map[string]any)
			acts = append(acts, mergedAction{v.id, pos.path, str(m["action"]), merged})
		}
	}
	return els, acts
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
