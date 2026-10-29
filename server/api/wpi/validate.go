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
	tablePattern  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*){0,2}$`)
	sizePattern   = regexp.MustCompile(`^(auto|elastic|[0-9]+(\.[0-9]+)?(px|rem|em|%))$`)
	// A page's size is set, not computed by a layouter, so it cannot be elastic.
	// A page length (design: interpreter-page §4): a number with an optional
	// unit (px when omitted), spaces allowed before the unit; width and height
	// also accept auto.
	pageLengthPattern = regexp.MustCompile(`^([0-9]+(\.[0-9]+)?)\s*(px|rem|em|%)?$`)
	colorPattern      = regexp.MustCompile(`^(auto|#([0-9a-fA-F]{3,4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})|(rgb|rgba|hsl|hsla)\([0-9.,%\s/-]+\)|[a-zA-Z]{3,30})$`)
	flowGapPattern    = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?(px|rem)$`)
	localePattern     = regexp.MustCompile(`^[a-z]{2,3}(-[a-z0-9]{2,8})*$`)
)

var elementSizeAttrs = []string{"width", "height", "min_width", "max_width"}

// pageSizeAttrs are the page's lengths; only width and height accept auto.
var pageSizeAttrs = []string{"width", "height", "min_width", "min_height", "max_width", "max_height", "left_margin", "right_margin", "top_margin", "bottom_margin"}

var bindingModes = []string{"read-only", "read-write"}

// codeAttrs lists the instance attributes a class's code defines: required
// ones the code cannot work without, and optional ones.
type codeAttrs struct{ required, optional []string }

func (a codeAttrs) defines(attr string) bool {
	return slices.Contains(a.required, attr) || slices.Contains(a.optional, attr)
}

// codeModules lists the instance attributes each element code module defines,
// keyed by code module ID: the name under which the frontend registers the code
// that draws it. An element class names its module in code_module_id, and
// several classes may share one (devdoc 2026102601 §3.5.2; each module is
// documented in KnowledgeStore doc-repo/code_modules). Every module also defines
// the size attributes.
var codeModules = map[string]codeAttrs{
	"text":   {nil, []string{"text", "data_binding"}},
	"input":  {[]string{"label", "data_binding", "binding_mode"}, []string{"input_type"}},
	"select": {[]string{"label", "data_binding", "binding_mode", "options"}, nil},
	"button": {[]string{"label", "on_click"}, nil},
	"list":   {[]string{"source", "presentation", "columns"}, []string{"on_row_click", "pagination"}},
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
// code_module_id is set only in element class rows.
var classAttrs = []string{"descriptor_id", "schema_version", "description", "implement_notes", "code_module_id"}

// actionHolding attributes hold action instances. A class row cannot give them
// defaults, so every action instance is written in a page (design D5).
var actionHolding = []string{"on_click", "on_row_click", "source", "items", "input"}

var (
	triggerClasses = []string{"save", "open-page", "close-page", "save-page"}
	inputClasses   = []string{"load", "query", "load-file"}
	sourceClasses  = []string{"load", "query"}
	saveClass      = []string{"save"}
)

// elementCode returns the instance attributes an element code module defines.
func elementCode(module string) (codeAttrs, bool) {
	a, ok := codeModules[module]
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
	v.schemaVersion(body)
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

func (v *validator) schemaVersion(body map[string]any) {
	want := schemaVersionOf(v.kind)
	if sv, _ := body["schema_version"].(string); sv != want {
		v.errf("/schema_version", "schema_version %q is not supported by this interpreter for %s descriptors (supports %q)", sv, v.kind, want)
	}
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

// List width and height also accept non-negative integer pixel values.
func (v *validator) listSize(obj map[string]any, key, path string) {
	val, present := obj[key]
	if !present {
		return
	}
	if _, ok := val.(string); ok {
		v.size(obj, key, path)
		return
	}
	if n, ok := asInt(val); !ok || n < 0 {
		v.errf(path+"/"+key, "must be a size string or a non-negative integer in pixels")
	}
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
		"page_name", "description", "binding_mode", "layouter_config", "access_controls", "keywords", "page_tags",
		"page_policy", "page_style", "hori_align", "vert_align", "bkgd_color", "frgd_color",
		"width", "height", "min_width", "min_height", "max_width", "max_height",
		"left_margin", "right_margin", "top_margin", "bottom_margin")
	v.oneOf(b, "layouter", "", layouters)
	if config, present := b["layouter_config"]; present {
		if m, ok := v.object(config, "/layouter_config"); ok {
			v.keys(m, "/layouter_config", nil, "column_gap", "row_gap", "horizontal_alignment", "vertical_alignment", "rows_alignment")
			v.matching(m, "column_gap", "/layouter_config", flowGapPattern)
			v.matching(m, "row_gap", "/layouter_config", flowGapPattern)
			v.oneOf(m, "horizontal_alignment", "/layouter_config", []string{"start", "center", "end", "space-between"})
			v.oneOf(m, "vertical_alignment", "/layouter_config", []string{"top", "center", "bottom", "stretch"})
			v.oneOf(m, "rows_alignment", "/layouter_config", []string{"top", "center", "bottom", "space-between"})
		}
	}
	v.text(b, "page_name", "")
	v.textLength(b, "page_name", 100)
	v.text(b, "description", "")
	v.textLength(b, "description", 1000)
	v.stringList(b, "access_controls", 0)
	v.stringList(b, "keywords", 200)
	v.stringList(b, "page_tags", 200)
	// page_policy and page_style are reserved: accepted and ignored.
	v.str(b, "page_policy", "")
	v.str(b, "page_style", "")
	v.oneOf(b, "hori_align", "", []string{"left", "center", "right"})
	v.oneOf(b, "vert_align", "", []string{"top", "center", "bottom"})
	for _, k := range []string{"bkgd_color", "frgd_color"} {
		v.matching(b, k, "", colorPattern)
	}
	for _, k := range pageSizeAttrs {
		v.pageLength(b, k, k == "width" || k == "height")
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

// classRow checks an active element or action class row (design D2). An action
// class row holds instance attributes its code defines, each with a default or
// null; every attribute the code requires must be there, and attributes that
// hold actions cannot have defaults. An element class row is checked by
// elementClassRow.
func (v *validator) classRow(b map[string]any) {
	v.text(b, "description", "")
	v.text(b, "implement_notes", "")
	if v.kind == KindElement {
		v.elementClassRow(b)
		return
	}
	code, known := actionAttrs[v.id]
	if !known {
		v.errf("", "%s class %q has no code in this interpreter", v.kind, v.id)
		return
	}
	defaults := map[string]any{}
	for _, k := range slices.Sorted(maps.Keys(b)) {
		switch {
		case k == "code_module_id":
			v.errf("/"+k, "an action class's ID names its code, so it has no %q", k)
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
	v.action(v.id, defaults, "")
}

// elementClassRow checks an active element class row (format 0.4): it names a
// registered code module, gives every attribute the module requires a default
// or null, and the defaults of attributes the module defines are valid. It may
// name attributes the module does not define; classWarnings reports them
// (devdoc 2026102601 §3.5.2).
func (v *validator) elementClassRow(b map[string]any) {
	module, code, registered := v.codeModule(b, true)
	defaults, given, _ := v.elementClassAttrs(b)
	if !registered {
		return
	}
	for _, k := range code.required {
		if !given[k] {
			v.errf("", "the class must give %q a default_value or null, because its code module %q requires it", k, module)
		}
	}
	v.element(module, defaults, "")
}

// codeModule checks an element class row's code_module_id: an active class
// must name a registered code module; a draft may leave it out or name one whose
// code is not written yet, so only its form is checked. It returns the module
// and, when it is registered, the attributes it defines.
func (v *validator) codeModule(b map[string]any, active bool) (module string, code codeAttrs, registered bool) {
	val, present := b["code_module_id"]
	if !present {
		if active {
			v.errf("/code_module_id", "missing attribute %q: an active element class names its code module", "code_module_id")
		}
		return "", codeAttrs{}, false
	}
	module, _ = val.(string)
	if !idPattern.MatchString(module) {
		v.errf("/code_module_id", "must be a code module ID matching %s", idPattern)
		return "", codeAttrs{}, false
	}
	code, registered = elementCode(module)
	if active && !registered {
		v.errf("/code_module_id", "code module %q has no code in this interpreter", module)
	}
	return module, code, registered
}

// classWarnings reports the instance attributes an element class row names that
// its code module does not define: they are stored, but have no effect until
// the module is changed (devdoc 2026102601 §3.5.2). A row whose module is not
// registered has none.
func classWarnings(id string, body map[string]any) []Issue {
	module := str(body["code_module_id"])
	code, registered := elementCode(module)
	if !registered {
		return nil
	}
	var out []Issue
	for _, k := range slices.Sorted(maps.Keys(body)) {
		if !slices.Contains(classAttrs, k) && !code.defines(k) {
			out = append(out, Issue{KindElement, id, "/" + k, fmt.Sprintf("code module %q does not support attribute %q, so it has no effect", module, k)})
		}
	}
	return out
}

// elementClassAttrs checks the instance attributes of an element class row
// (format 0.4): each has a well-formed name and is an object with an optional
// help_tip (text for authors), an optional default_value, where null means
// every instance must set it and leaving it out means the attribute is optional
// with no default, and an optional instance_modifiable. false makes the
// attribute fixed: its default_value, which may not be null, is the value of
// every instance. It returns the non-null defaults, the names that have a
// default_value (null included) and the fixed ones.
func (v *validator) elementClassAttrs(b map[string]any) (defaults map[string]any, given, fixed map[string]bool) {
	defaults, given, fixed = map[string]any{}, map[string]bool{}, map[string]bool{}
	for _, k := range slices.Sorted(maps.Keys(b)) {
		p := "/" + k
		switch {
		case slices.Contains(classAttrs, k):
			continue
		case k == "element" || !segmentPattern.MatchString(k):
			v.errf(p, "%q cannot be an attribute name", k)
			continue
		}
		attr, ok := b[k].(map[string]any)
		if !ok {
			v.errf(p, "must be an object with an optional %q, %q and %q", "help_tip", "default_value", "instance_modifiable")
			continue
		}
		v.keys(attr, p, nil, "help_tip", "default_value", "instance_modifiable")
		v.text(attr, "help_tip", p)
		v.boolean(attr, "instance_modifiable", p)
		dv, has := attr["default_value"]
		if isFixed(b, k) {
			fixed[k] = true
			switch {
			case slices.Contains(actionHolding, k):
				v.errf(p, "attribute %q holds actions, so it cannot be fixed", k)
			case dv == nil:
				v.errf(p, "a fixed attribute needs a default_value other than null: it is the value of every instance")
			}
		}
		if !has {
			continue
		}
		given[k] = true
		switch {
		case dv == nil:
		case slices.Contains(actionHolding, k):
			v.errf(p+"/default_value", "attribute %q holds actions, so it cannot have a default; use null or leave it out", k)
		default:
			defaults[k] = dv
		}
	}
	return defaults, given, fixed
}

// isFixed reports whether an element class row fixes attribute k for all its
// instances ("instance_modifiable": false).
func isFixed(row map[string]any, k string) bool {
	attr, _ := row[k].(map[string]any)
	return attr["instance_modifiable"] == false
}

// fixedValues returns the attributes an element class row fixes, with their values.
func fixedValues(row map[string]any) map[string]any {
	out := map[string]any{}
	for k := range row {
		if isFixed(row, k) {
			attr, _ := row[k].(map[string]any)
			out[k] = attr["default_value"]
		}
	}
	return out
}

// ValidateDraftClass checks an element class row created as a draft, before its
// code may exist (OpenSpec wpi-create-element-class, design D4).
func ValidateDraftClass(body map[string]any) []Issue {
	return validateElementClass(body, true)
}

// validateElementClass checks an element class row in the authoring format,
// independently of which attributes its code module defines: names, attribute
// objects, help tips, fixed attributes, action defaults and the defaults of
// attributes every class shares. A draft needs a description and may leave out
// its code module; an active row may omit a description but must name a
// registered code module (OpenSpec wpi-code-modules).
func validateElementClass(body map[string]any, draft bool) []Issue {
	id, _ := body["descriptor_id"].(string)
	v := &validator{kind: KindElement, id: id}
	if !idPattern.MatchString(id) {
		v.errf("/descriptor_id", "descriptor ID %q must match %s", id, idPattern)
	}
	v.schemaVersion(body)
	if _, ok := body["description"]; draft && !ok {
		v.errf("/description", "a draft class needs a description")
	}
	v.text(body, "description", "")
	v.text(body, "implement_notes", "")
	v.codeModule(body, !draft)
	defaults, _, _ := v.elementClassAttrs(body)
	v.sharedAttrs(defaults, "")
	return v.issues
}

// sharedAttrs checks the attributes many element code modules share: sizes,
// label, data_binding and binding_mode.
func (v *validator) sharedAttrs(e map[string]any, path string) {
	for _, k := range elementSizeAttrs {
		if k == "width" || k == "height" {
			// The module check rejects integer sizes for modules other than list.
			v.listSize(e, k, path)
		} else {
			v.size(e, k, path)
		}
	}
	v.text(e, "label", path)
	v.binding(e, path)
	v.oneOf(e, "binding_mode", path, bindingModes)
}

// elementInstance checks a page entry {"element": class, ...} as far as it can
// without the class row: it names a class, sets no class attribute, and the
// attributes many classes share are valid. pageInstances checks it against its
// class row and code module.
func (v *validator) elementInstance(m map[string]any, path string) {
	if _, ok := v.matching(m, "element", path, idPattern); !ok {
		if _, present := m["element"]; !present {
			v.errf(path, "missing attribute %q", "element")
		}
		return
	}
	v.noClassAttrs(m, path)
	v.sharedAttrs(instanceAttrs(m, "element"), path)
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

// element checks the values of the attributes an element instance or class row
// sets that its code module defines; the others have no effect, so they are not
// checked here. Which attributes an instance may set, and whether required ones
// are there, is checked against its class (elementAgainstClass,
// elementComplete). Action instances inside are checked through actionPositions.
func (v *validator) element(module string, all map[string]any, path string) {
	code, _ := elementCode(module)
	e := map[string]any{}
	for k, val := range all {
		if code.defines(k) {
			e[k] = val
		}
	}
	for _, k := range elementSizeAttrs {
		if module == "list" && (k == "width" || k == "height") {
			v.listSize(e, k, path)
		} else {
			v.size(e, k, path)
		}
	}
	switch module {
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
		v.oneOf(e, "pagination", path, []string{"yes", "no"})
	case "menu":
		if items, ok := e["items"]; ok {
			v.menuItems(items, path+"/items", map[string]bool{})
		}
		v.str(e, "initial_item", path)
	}
}

// elementComplete checks a merged element instance: every attribute its code
// module requires is there, and the rules that span attributes hold.
func (v *validator) elementComplete(module string, merged map[string]any, path string) {
	code, _ := elementCode(module)
	v.required(merged, path, code.required)
	switch module {
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

// textLength limits a text, or each language of one, to max characters.
func (v *validator) textLength(obj map[string]any, key string, max int) {
	var texts []string
	switch t := obj[key].(type) {
	case string:
		texts = []string{t}
	case map[string]any:
		for _, lang := range slices.Sorted(maps.Keys(t)) {
			if s, ok := t[lang].(string); ok {
				texts = append(texts, s)
			}
		}
	}
	for _, s := range texts {
		if n := len([]rune(s)); n > max {
			v.errf("/"+key, "is %d characters; the limit is %d", n, max)
			return
		}
	}
}

// stringList checks an optional array of non-empty strings; max 0 means no limit.
func (v *validator) stringList(obj map[string]any, key string, max int) {
	if _, present := obj[key]; !present || obj[key] == nil {
		return
	}
	list, ok := v.array(obj[key], "/"+key, false)
	if !ok {
		return
	}
	if max > 0 && len(list) > max {
		v.errf("/"+key, "has %d items; the limit is %d", len(list), max)
	}
	for i, item := range list {
		if s, ok := item.(string); !ok || s == "" {
			v.errf("/"+key+"/"+strconv.Itoa(i), "must be a non-empty string")
		}
	}
}

// pageLength checks a page length: a non-negative number with an optional unit,
// and for % a value within [0, 100].
func (v *validator) pageLength(obj map[string]any, key string, allowAuto bool) {
	s, ok := v.str(obj, key, "")
	if !ok || (allowAuto && s == "auto") {
		return
	}
	m := pageLengthPattern.FindStringSubmatch(s)
	if m == nil {
		v.errf("/"+key, "%q must be a non-negative number with an optional unit (px, rem, em, %%)%s", s, map[bool]string{true: " or auto"}[allowAuto])
		return
	}
	if n, _ := strconv.ParseFloat(m[1], 64); m[3] == "%" && n > 100 {
		v.errf("/"+key, "%q must be within 0%% and 100%%", s)
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
// (design D5), including open-page inputs nested in them. The names of the
// element attributes that hold actions are reserved (actionHolding), so it needs
// neither class rows nor code modules (OpenSpec wpi-code-modules, design D5). It
// accepts pages that fail validation, so callers can use it on any stored page.
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
		path := "/elements/" + strconv.Itoa(i)
		add := func(attr string, allowed, roots []string) {
			if val, ok := m[attr]; ok {
				out = append(out, actionPos{val, path + "/" + attr, allowed, roots, false})
			}
		}
		add("on_click", triggerClasses, []string{RootInputDoc})
		add("source", sourceClasses, []string{RootInputDoc})
		add("on_row_click", triggerClasses, []string{RootInputDoc, RootRecord})
		out = append(out, itemPositions(m["items"], path+"/items")...)
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

// classDefaults returns a class row's non-null defaults and the attributes
// whose default is null, which every instance must set. An element class row
// (format 0.4) holds each instance attribute as {help_tip, default_value,
// instance_modifiable}; an action class row holds the default itself.
func classDefaults(kind Kind, row map[string]any) (defaults map[string]any, required []string) {
	defaults = map[string]any{}
	for _, k := range slices.Sorted(maps.Keys(row)) {
		if slices.Contains(classAttrs, k) {
			continue
		}
		val := row[k]
		if kind == KindElement {
			attr, _ := val.(map[string]any)
			dv, has := attr["default_value"]
			if !has {
				continue
			}
			val = dv
		}
		if val == nil {
			required = append(required, k)
		} else {
			defaults[k] = val
		}
	}
	return defaults, required
}

// mergeInstance overlays an instance's own attributes on its class row's
// non-null defaults (devdoc 2026102601 §3.5.1), then, for an element, the
// values the class fixes, which an instance cannot change.
func mergeInstance(kind Kind, row, own map[string]any) map[string]any {
	out, _ := classDefaults(kind, row)
	maps.Copy(out, own)
	if kind == KindElement {
		maps.Copy(out, fixedValues(row))
	}
	return out
}

// againstClass checks an instance's own attributes against its active class
// row and returns the merged instance and the row. Every attribute whose
// default is null must be set by the instance. An element instance may set only
// the attributes its class names, plus the size attributes every element has,
// and none that its class fixes (OpenSpec wpi-code-modules, design D4, D5).
func (v *validator) againstClass(kind Kind, cls string, own map[string]any, path string) (map[string]any, map[string]any, bool) {
	row, ok := v.classes[kind][cls]
	if !ok {
		key := "element"
		if kind == KindAction {
			key = "action"
		}
		v.errf(path+"/"+key, "%s class %q does not exist", kind, cls)
		return nil, nil, false
	}
	if kind == KindElement {
		for _, k := range slices.Sorted(maps.Keys(own)) {
			_, named := row.Body[k]
			switch {
			case isFixed(row.Body, k):
				v.errf(path+"/"+k, "attribute %q is fixed by element class %q; an instance cannot set it", k, cls)
			case !named && !slices.Contains(elementSizeAttrs, k):
				v.errf(path+"/"+k, "element class %q has no attribute %q", cls, k)
			}
		}
	}
	_, required := classDefaults(kind, row.Body)
	for _, k := range required {
		if _, set := own[k]; !set {
			v.errf(path, "attribute %q has no default in %s class %q, so this instance must set it", k, kind, cls)
		}
	}
	return mergeInstance(kind, row.Body, own), row.Body, true
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
	merged, _, ok := v.againstClass(KindAction, cls, instanceAttrs(m, "action"), pos.path)
	if !ok {
		return nil
	}
	v.required(merged, pos.path, actionAttrs[cls].required)
	return merged
}

// withPageDefaults gives an element instance that does not set binding_mode
// the page's, when its class row names binding_mode and does not fix it, so the
// order is instance, page, class (devdoc 2026102601 §3.2.1, design D4; OpenSpec
// wpi-code-modules, design D6). Same rule as resolveEntry in web/src/lib/wpi.
func withPageDefaults(page, row, own map[string]any) map[string]any {
	mode, set := page["binding_mode"]
	_, ownMode := own["binding_mode"]
	_, named := row["binding_mode"]
	if set && !ownMode && named && !isFixed(row, "binding_mode") {
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
// D3, second stage) and the code modules those rows name, and returns them
// merged with their classes.
func (v *validator) pageInstances(page map[string]any) ([]mergedElement, []mergedAction) {
	var els []mergedElement
	list, _ := page["elements"].([]any)
	for i, e := range list {
		m, _ := e.(map[string]any)
		cls, path := str(m["element"]), "/elements/"+strconv.Itoa(i)
		own := withPageDefaults(page, v.classes[KindElement][cls].Body, instanceAttrs(m, "element"))
		if merged, row, ok := v.againstClass(KindElement, cls, own, path); ok {
			module := str(row["code_module_id"])
			v.element(module, own, path)
			v.elementComplete(module, merged, path)
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
