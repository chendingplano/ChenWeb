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
	paneIDPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
)

var elementSizeAttrs = []string{"width", "height", "min_width", "max_width"}

// pageSizeAttrs are the page's lengths; only width and height accept auto.
var pageSizeAttrs = []string{"width", "height", "min_width", "min_height", "max_width", "max_height"}

// paneAttrs apply to every pane; leafPaneAttrs concern how a leaf places content, so they
// apply to leaf panes only (devdoc 2026102601 §3.2.2).
var (
	paneAttrs     = []string{"width", "height", "min_width", "min_height", "max_width", "max_height", "resizable", "background_color", "foreground_color", "border_thickness", "border_light_color", "border_dark_color", "left_margin", "right_margin", "top_margin", "bottom_margin"}
	leafPaneAttrs = []string{"hori_align", "vert_align", "hori_scrollbar", "vert_scrollbar"}
	scrollbars    = []string{"auto", "hide"}
)

// pageLayouterModules are the code modules that draw pane trees, and
// elementLayouterModules the parameters each element layouter code module takes
// in its config, keyed by code module ID (OpenSpec wpi-pages-own-panes, design
// D6). The frontend registers the code under the same IDs.
var (
	pageLayouterModules    = []string{"page-layouter"}
	elementLayouterModules = map[string][]string{
		"flow-layouter": {"left_margin", "right_margin", "top_margin", "bottom_margin", "column_gap", "row_gap", "horizontal_alignment", "vertical_alignment", "rows_alignment", "min_row_height", "max_row_height", "min_elem_width", "max_elem_width", "max_num_elems", "row_separator", "elem_separator"},
	}
)

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
	"text":   {nil, textAttributes},
	"input":  {[]string{"label", "data_binding", "binding_mode"}, []string{"input_type", "placeholder", "help_text", "help_tip", "required", "max_length", "validation", "autocomplete", "label_placement", "label_horizontal_gap", "label_vertical_gap", "label_horizontal_align", "label_vertical_align", "label_max_width", "label_max_lines", "label_font_family", "label_font_size", "label_color", "label_font_weight", "label_font_style"}},
	"select": {[]string{"label", "data_binding", "binding_mode", "options"}, nil},
	"button": {[]string{"label", "on_click"}, []string{"background_color"}},
	"list":   {[]string{"source", "presentation", "columns"}, []string{"on_row_click", "pagination", "background_color", "font_color", "odd_row_background_color", "even_row_background_color"}},
	"menu":   {[]string{"items"}, []string{"initial_item"}},
	// chat_attributes.go
	"chat-composer": {nil, chatComposerAttributes},
	"chat-panel":    {nil, chatPanelAttributes},
	// value_editor_attributes.go
	"value-editor": {[]string{"editor", "label", "binding_mode"}, valueEditorAttributes},
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
var classAttrs = []string{"descriptor_id", "schema_version", "description", "implement_notes", "code_module_id", "attribute_definitions"}

// copyKey is the attribute in which an instance or layouter copy holds the
// whole class row it was copied from (devdoc 2026102601 §3.5.1), so no class row
// may name an attribute "class".
const copyKey = "class"

// actionHolding attributes hold action instances. A class row cannot give them
// defaults, so every action instance is written in a page (design D5).
var actionHolding = []string{"on_click", "on_row_click", "source", "items", "input"}

var (
	triggerClasses = []string{"save", "open-page", "close-page", "save-page"}
	inputClasses   = []string{"load", "query", "load-file"}
	sourceClasses  = []string{"load", "query"}
	saveClass      = []string{"save"}
)

// elementCode returns the instance attributes an element code module defines:
// its own, the sizes, and validators for the modules that take them
// (validator_classes.go).
func elementCode(module string) (codeAttrs, bool) {
	a, ok := codeModules[module]
	a.optional = append(slices.Clone(a.optional), elementSizeAttrs...)
	if acceptsValidators(module) {
		a.optional = append(a.optional, "validators")
	}
	return a, ok
}

type validator struct {
	allowIncomplete bool
	kind            Kind
	id              string
	issues          []Issue
	// els and acts collect a page's instances merged with their class copies.
	els  []mergedElement
	acts []mergedAction
}

func (v *validator) errf(path, format string, a ...any) {
	v.issues = append(v.issues, Issue{Kind: v.kind, DescriptorID: v.id, Path: path, Message: fmt.Sprintf(format, a...)})
}

// ValidateDescriptor checks one stored descriptor completely, as far as that is
// possible without the database: structure, that every class it names has code,
// and a page's instances and layouter copies against the class copies they hold.
// BuildBundle follows references to other descriptors and checks tables, stored
// sources, code modules and panes.
func ValidateDescriptor(kind Kind, id string, body map[string]any) []Issue {
	return newValidator(kind, id, body).issues
}

// validatePage checks a page and returns its issues and its instances merged
// with their class copies.
func validatePage(id string, body map[string]any) ([]Issue, []mergedElement, []mergedAction) {
	v := newValidator(KindPage, id, body)
	return v.issues, v.els, v.acts
}

func newValidator(kind Kind, id string, body map[string]any) *validator {
	return newValidatorMode(kind, id, body, false)
}

func newValidatorMode(kind Kind, id string, body map[string]any, allowIncomplete bool) *validator {
	v := &validator{kind: kind, id: id, allowIncomplete: allowIncomplete}
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
	case KindPageLayouter, KindElementLayouter:
		v.layouterClass(body)
	case KindValidator:
		v.validatorClass(body)
	default:
		v.errf("", "unknown descriptor kind %q", kind)
	}
	return v
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

// app checks an app descriptor: it names, by descriptor ID, the page it opens in
// its page area, and may carry the text authors search. Pages are not embedded.
func (v *validator) app(b map[string]any) {
	v.keys(b, "", []string{"descriptor_id", "schema_version", "initial_page"}, "app_name", "app_description", "keywords", "tags")
	v.matching(b, "initial_page", "", idPattern)
	v.text(b, "app_name", "")
	v.textLength(b, "app_name", 100)
	v.text(b, "app_description", "")
	v.textLength(b, "app_description", 1000)
	v.stringList(b, "keywords", 200)
	v.stringList(b, "tags", 200)
}

func (v *validator) page(b map[string]any) {
	v.keys(b, "", []string{"descriptor_id", "schema_version", "page_layouter"}, "panels", "input", "save_action", "refresh",
		"page_name", "description", "binding_mode", "access_controls", "keywords", "page_tags",
		"page_policy", "page_style", "page_image", "hori_align", "vert_align", "hori_scrollbar", "vert_scrollbar", "bkgd_color", "frgd_color",
		"width", "height", "min_width", "min_height", "max_width", "max_height")
	var leaves map[string]bool
	if pl, ok := b["page_layouter"]; ok {
		leaves = v.pageLayouterCopy(pl, "/page_layouter")
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
	v.matching(b, "page_image", "", pageImageName)
	v.oneOf(b, "hori_align", "", []string{"left", "center", "right"})
	v.oneOf(b, "vert_align", "", []string{"top", "center", "bottom"})
	v.oneOf(b, "hori_scrollbar", "", scrollbars)
	v.oneOf(b, "vert_scrollbar", "", scrollbars)
	for _, k := range []string{"bkgd_color", "frgd_color"} {
		if raw, ok := b[k]; ok {
			if _, pair := raw.(map[string]any); pair {
				v.surfaceColorPair(raw, "/"+k)
			} else {
				v.matching(b, k, "", colorPattern)
			}
		}
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
	if panels, ok := b["panels"]; ok {
		v.panels(b, panels, leaves)
		v.oneChatComposer()
	}
	for _, pos := range actionPositions(b) {
		if merged := v.actionInstance(pos); merged != nil {
			v.acts = append(v.acts, mergedAction{v.id, pos.path, str(pos.val.(map[string]any)["action"]), merged})
		}
	}
}

// panels checks a page's panels: each is keyed by a leaf pane of the page's
// page layouter (leaves is nil when the page layouter is invalid) and has an
// element layouter copy and a list of element instances (devdoc 2026102601 §3.3).
func (v *validator) panels(page map[string]any, val any, leaves map[string]bool) {
	m, ok := v.object(val, "/panels")
	if !ok {
		return
	}
	for _, paneID := range slices.Sorted(maps.Keys(m)) {
		path := "/panels/" + paneID
		if leaves != nil && !leaves[paneID] {
			v.errf(path, "%q is not a leaf pane of this page's page layouter", paneID)
		}
		p, ok := v.object(m[paneID], path)
		if !ok {
			continue
		}
		v.keys(p, path, []string{"elem_layouter"}, "elem_list", "background_color", "foreground_color")
		v.surfaceColors(p, path)
		if el, ok := p["elem_layouter"]; ok {
			v.elementLayouterCopy(el, path+"/elem_layouter")
		}
		list, ok := p["elem_list"]
		if !ok {
			continue
		}
		items, ok := v.array(list, path+"/elem_list", false)
		if !ok {
			continue
		}
		for i, e := range items {
			ep := path + "/elem_list/" + strconv.Itoa(i)
			if em, ok := v.object(e, ep); ok {
				v.elementInstance(page, em, ep)
			}
		}
	}
}

// pageLayouterCopy checks a page's page layouter copy, {layouter, class, root?}:
// the class copy is a valid page layouter class row, and the page's own root,
// when set, replaces the copy's whole (OpenSpec wpi-pages-own-panes, design D2).
// It returns the leaf pane IDs of the effective tree, or nil when it is invalid.
func (v *validator) pageLayouterCopy(val any, path string) map[string]bool {
	m, ok := v.object(val, path)
	if !ok {
		return nil
	}
	v.keys(m, path, []string{"layouter", copyKey}, "root", instanceIDKey)
	row, ok := v.classCopy(KindPageLayouter, m, "layouter", path)
	if root, present := m["root"]; present {
		return v.paneTree(root, path+"/root")
	}
	if !ok {
		return nil
	}
	leaves, _ := paneLeaves(row["root"])
	return leaves
}

// elementLayouterCopy checks a panel's element layouter copy,
// {layouter, class, config?}: the class copy is a valid element layouter class
// row, and the panel's own config, when set, replaces the copy's whole.
func (v *validator) elementLayouterCopy(val any, path string) {
	m, ok := v.object(val, path)
	if !ok {
		return
	}
	v.keys(m, path, []string{"layouter", copyKey}, "config", instanceIDKey)
	row, ok := v.classCopy(KindElementLayouter, m, "layouter", path)
	if config, present := m["config"]; present && ok {
		v.layouterConfig(str(row["code_module_id"]), config, path+"/config")
	}
}

// layouterClass checks a page layouter class row (a code module and a pane
// tree, root) or an element layouter class row (a code module and its default
// parameters, config).
func (v *validator) layouterClass(b map[string]any) {
	v.attributeDefinitions(b)
	registry, tree := pageLayouterModules, "root"
	if v.kind == KindElementLayouter {
		registry, tree = slices.Sorted(maps.Keys(elementLayouterModules)), "config"
	}
	v.keys(b, "", []string{"descriptor_id", "schema_version", "code_module_id", tree}, "description", "attribute_definitions")
	v.text(b, "description", "")
	module, ok := v.matching(b, "code_module_id", "", idPattern)
	if ok && !slices.Contains(registry, module) {
		v.errf("/code_module_id", "code module %q has no %s code in this interpreter", module, v.kind)
		ok = false
	}
	val, present := b[tree]
	switch {
	case !present:
	case v.kind == KindPageLayouter:
		v.paneTree(val, "/root")
	case ok:
		v.layouterConfig(module, val, "/config")
	}
}

// layouterConfig checks the parameters of an element layouter code module.
func (v *validator) layouterConfig(module string, val any, path string) {
	m, ok := v.object(val, path)
	if !ok {
		return
	}
	v.keys(m, path, nil, elementLayouterModules[module]...)
	if module == "flow-layouter" {
		v.matching(m, "column_gap", path, flowGapPattern)
		v.matching(m, "row_gap", path, flowGapPattern)
		for _, key := range []string{"left_margin", "right_margin", "top_margin", "bottom_margin", "min_row_height", "max_row_height", "min_elem_width", "max_elem_width"} {
			v.matching(m, key, path, flowGapPattern)
		}
		if value, present := m["max_num_elems"]; present {
			if n, ok := asInt(value); !ok || n < 0 {
				v.errf(path+"/max_num_elems", "must be a non-negative integer")
			}
		}
		separators := []string{"none", "thin_solid_line", "norm_solid_line", "bold_solid_line", "thin_dash_line", "norm_dash_line", "bold_dash_line"}
		v.oneOf(m, "row_separator", path, separators)
		v.oneOf(m, "elem_separator", path, separators)
		v.oneOf(m, "horizontal_alignment", path, []string{"start", "center", "end", "space-between"})
		v.oneOf(m, "vertical_alignment", path, []string{"top", "center", "bottom", "stretch"})
		v.oneOf(m, "rows_alignment", path, []string{"top", "center", "bottom", "space-between"})
	}
}

// paneTree checks a pane tree with exactly one root and returns its leaf pane
// IDs (devdoc 2026102601 §3.2.1, §3.2.2).
func (v *validator) paneTree(val any, path string) map[string]bool {
	ids, leaves := map[string]bool{}, map[string]bool{}
	v.pane(val, path, ids, leaves)
	return leaves
}

func (v *validator) pane(val any, path string, ids, leaves map[string]bool) {
	p, ok := v.object(val, path)
	if !ok {
		return
	}
	v.keys(p, path, []string{"pane_id"}, append(append([]string{"split", "children"}, paneAttrs...), leafPaneAttrs...)...)
	id, ok := v.matching(p, "pane_id", path, paneIDPattern)
	if ok {
		if ids[id] {
			v.errf(path+"/pane_id", "pane ID %q is used twice in this page", id)
		}
		ids[id] = true
	}
	for _, k := range []string{"width", "height"} {
		v.length(p, k, path, lengthKind{auto: true, elastic: true, percent: true})
	}
	for _, k := range []string{"min_width", "min_height", "max_width", "max_height"} {
		v.length(p, k, path, lengthKind{percent: true})
	}
	v.boolean(p, "resizable", path)
	v.surfaceColors(p, path)
	if raw, ok := p["border_thickness"]; ok && raw != nil {
		value, valid := raw.(string)
		if !valid || !regexp.MustCompile(`^[0-9]+(\.[0-9]+)?px$`).MatchString(value) {
			v.errf(path+"/border_thickness", "must be a non-negative pixel length")
		}
	}
	for _, key := range []string{"border_light_color", "border_dark_color"} {
		if raw, ok := p[key]; ok && raw != nil {
			value, valid := raw.(string)
			if !valid || value == "auto" || !colorPattern.MatchString(value) {
				v.errf(path+"/"+key, "must be a CSS color")
			}
		}
	}
	for _, k := range []string{"left_margin", "right_margin", "top_margin", "bottom_margin"} {
		v.length(p, k, path, lengthKind{})
	}
	if _, split := p["split"]; split {
		v.oneOf(p, "split", path, []string{"rows", "columns"})
		for _, k := range leafPaneAttrs {
			if _, set := p[k]; set {
				v.errf(path+"/"+k, "%q applies to leaf panes only", k)
			}
		}
		children, ok := v.array(p["children"], path+"/children", false)
		if ok && len(children) < 2 {
			v.errf(path+"/children", "a split pane needs two or more children")
		}
		for i, c := range children {
			v.pane(c, path+"/children/"+strconv.Itoa(i), ids, leaves)
		}
		return
	}
	if _, has := p["children"]; has {
		v.errf(path+"/children", "a pane without %q has no children", "split")
	}
	if ok {
		leaves[id] = true
	}
	v.oneOf(p, "hori_align", path, []string{"left", "center", "right"})
	v.oneOf(p, "vert_align", path, []string{"top", "center", "bottom"})
	v.oneOf(p, "hori_scrollbar", path, scrollbars)
	v.oneOf(p, "vert_scrollbar", path, scrollbars)
}

// paneLeaves returns the leaf pane IDs of a tree that is already known to be
// valid, and every pane ID in it.
func paneLeaves(root any) (leaves, all map[string]bool) {
	leaves, all = map[string]bool{}, map[string]bool{}
	var walk func(any)
	walk = func(val any) {
		p, _ := val.(map[string]any)
		id := str(p["pane_id"])
		all[id] = true
		children, split := p["children"].([]any)
		if !split {
			leaves[id] = true
		}
		for _, c := range children {
			walk(c)
		}
	}
	walk(root)
	return leaves, all
}

// effectiveRoot returns a page's pane tree: its page layouter's own root, else
// the root of its class copy.
func effectiveRoot(page map[string]any) map[string]any {
	pl, _ := page["page_layouter"].(map[string]any)
	if root, ok := pl["root"].(map[string]any); ok {
		return root
	}
	row, _ := pl[copyKey].(map[string]any)
	root, _ := row["root"].(map[string]any)
	return root
}

// classCopy checks the class copy an instance or layouter copy holds in
// "class": it is a valid class row of kind, checked as that kind's descriptor,
// and its descriptor_id equals the name in nameKey. Issues inside the copy are
// reported at their place in this descriptor. It returns the copy and whether
// it is valid.
func (v *validator) classCopy(kind Kind, m map[string]any, nameKey, path string) (map[string]any, bool) {
	name, ok := v.matching(m, nameKey, path, idPattern)
	if !ok {
		return nil, false
	}
	val, present := m[copyKey]
	if !present {
		return nil, false // reported by the caller's keys check
	}
	row, ok := v.object(val, path+"/"+copyKey)
	if !ok {
		return nil, false
	}
	if got := str(row["descriptor_id"]); got != name {
		v.errf(path+"/"+nameKey, "%s %q does not match the descriptor_id %q of its class copy", nameKey, name, got)
		return nil, false
	}
	issues := ValidateDescriptor(kind, name, row)
	for _, is := range issues {
		v.errf(path+"/"+copyKey+is.Path, "class copy: %s", is.Message)
	}
	return row, len(issues) == 0
}

// classRow checks an active element or action class row (design D2). An action
// class row holds instance attributes its code defines, each with a default or
// null; every attribute the code requires must be there, and attributes that
// hold actions cannot have defaults. An element class row is checked by
// elementClassRow.
func (v *validator) classRow(b map[string]any) {
	v.attributeDefinitions(b)
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
		case k == "element" || k == copyKey || !segmentPattern.MatchString(k):
			v.errf(p, "%q cannot be an attribute name", k)
			continue
		}
		attr, ok := b[k].(map[string]any)
		if !ok {
			v.errf(p, "must be an object with an optional %q, %q and %q", "help_tip", "default_value", "instance_modifiable")
			continue
		}
		v.keys(attr, p, nil, "help_tip", "default_value", "instance_modifiable", "scope", "value_editor")
		v.text(attr, "help_tip", p)
		v.boolean(attr, "instance_modifiable", p)
		v.oneOf(attr, "scope", p, []string{"class", "instance"})
		if scope, ok := attr["scope"].(string); ok {
			if flag, present := attr["instance_modifiable"].(bool); present && (scope == "class") == flag {
				v.errf(p+"/scope", "scope must agree with instance_modifiable")
			}
		}
		v.valueEditor(attr, p)
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
	if scope, ok := attr["scope"].(string); ok && (scope == "class" || scope == "instance") {
		return scope == "class"
	}
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
	v.attributeDefinitions(body)
	if !idPattern.MatchString(id) {
		v.errf("/descriptor_id", "descriptor ID %q must match %s", id, idPattern)
	}
	v.schemaVersion(body)
	if _, ok := body["description"]; draft && !ok {
		v.errf("/description", "a draft class needs a description")
	}
	v.text(body, "description", "")
	v.text(body, "implement_notes", "")
	module, _, _ := v.codeModule(body, !draft)
	defaults, _, _ := v.elementClassAttrs(body)
	v.sharedAttrs(defaults, "")
	if module == "value-editor" {
		v.valueEditorClassDefaults(defaults, draft)
	}
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

// elementInstance checks a panel entry {"element": class, "class": copy, ...}:
// it names its class, holds a valid copy of it, sets no class attribute, and is
// checked and merged against the copy, never the stored class (devdoc 2026102601
// §3.5.1). A page-wide binding_mode fills in first (withPageDefaults).
func (v *validator) elementInstance(page, m map[string]any, path string) {
	for _, k := range []string{"element", copyKey} {
		if _, present := m[k]; !present {
			v.errf(path, "missing attribute %q", k)
		}
	}
	v.noClassAttrs(m, path)
	own := instanceAttrs(m, "element")
	row, ok := v.classCopy(KindElement, m, "element", path)
	if !ok {
		return
	}
	cls := str(m["element"])
	module := str(row["code_module_id"])
	own = withPageDefaults(page, row, own)
	checked := maps.Clone(own)
	if module == "input" {
		if value, exists := checked["data_binding"]; exists && (value == nil || value == "") {
			delete(checked, "data_binding")
		}
	}
	v.sharedAttrs(checked, path)
	merged := v.againstCopy(KindElement, cls, row, own, path)
	v.element(module, checked, path)
	v.elementComplete(module, merged, path)
	v.els = append(v.els, mergedElement{cls, path, merged, row})
}

func (v *validator) noClassAttrs(m map[string]any, path string) {
	for _, k := range classAttrs {
		if _, ok := m[k]; ok {
			v.errf(path+"/"+k, "%q is a class attribute; an instance cannot set it", k)
		}
	}
}

// instanceIDKey is the attribute in which the editor records an instance's ID
// within its page: <descriptor_id>_<version>_<seqno> (devdoc 2026102601 §3.4).
// It identifies the copy, so it is not one of the copy's attributes. The
// frontend's dynamic_instance_id is never saved. The editor writes it when it
// copies the class onto the page; the server only accepts it.
const instanceIDKey = "instance_id"

// instanceAttrs returns an instance's own attributes: everything but its class
// name (classKey), its class copy, its instance_id and class attributes.
func instanceAttrs(m map[string]any, classKey string) map[string]any {
	out := map[string]any{}
	for k, val := range m {
		if k != classKey && k != copyKey && k != instanceIDKey && !slices.Contains(classAttrs, k) {
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
	if val, ok := e["validators"]; ok {
		v.validatorList(val, path+"/validators")
	}
	switch module {
	case "text":
		v.textAttributes(e, path)
	case "input", "select":
		v.text(e, "label", path)
		v.binding(e, path)
		v.oneOf(e, "binding_mode", path, bindingModes)
		v.oneOf(e, "input_type", path, []string{"text", "number", "email", "password", "tel", "url", "search"})
		if module == "input" {
			v.inputAttributes(e, path)
		}
		if _, ok := e["options"]; ok {
			v.options(e, path)
		}
	case "button":
		v.text(e, "label", path)
		if raw, exists := e["background_color"]; exists && raw != nil {
			if value, ok := raw.(string); ok {
				if value == "auto" || !colorPattern.MatchString(value) {
					v.errf(path+"/background_color", "invalid color %q", value)
				}
			} else {
				v.surfaceColorPair(raw, path+"/background_color")
			}
		}
	case "list":
		for _, key := range []string{"background_color", "font_color", "odd_row_background_color", "even_row_background_color"} {
			raw, exists := e[key]
			if !exists || raw == nil {
				continue
			}
			if value, ok := raw.(string); ok {
				if value == "auto" || !colorPattern.MatchString(value) {
					v.errf(path+"/"+key, "invalid color %q", value)
				}
			} else {
				v.surfaceColorPair(raw, path+"/"+key)
			}
		}
		v.listColumns(e, path)
		v.oneOf(e, "presentation", path, []string{"rows"})
		v.oneOf(e, "pagination", path, []string{"yes", "no"})
	case "menu":
		if items, ok := e["items"]; ok {
			v.menuItems(items, path+"/items", map[string]bool{})
		}
		v.str(e, "initial_item", path)
	case "chat-composer":
		v.chatComposerAttributes(e, path)
	case "chat-panel":
		v.chatPanelAttributes(e, path)
	case "value-editor":
		v.valueEditorAttributes(e, path)
	}
}

// inputAttributes validates the expanded input descriptor.
func (v *validator) inputAttributes(e map[string]any, path string) {
	for _, k := range []string{"placeholder", "help_text", "help_tip"} {
		v.text(e, k, path)
	}
	v.boolean(e, "required", path)
	for _, k := range []string{"label_horizontal_gap", "label_vertical_gap", "label_max_width", "label_font_size"} {
		v.matching(e, k, path, regexp.MustCompile(`^[0-9]+(\.[0-9]+)?px$`))
	}
	if size, ok := e["label_font_size"].(string); ok {
		if n, err := strconv.ParseFloat(strings.TrimSuffix(size, "px"), 64); err == nil && n <= 0 {
			v.errf(path+"/label_font_size", "must be positive")
		}
	}
	v.oneOf(e, "label_placement", path, []string{"left", "right", "top", "bottom"})
	v.oneOf(e, "label_horizontal_align", path, []string{"left", "center", "right"})
	v.oneOf(e, "label_vertical_align", path, []string{"top", "center", "bottom"})
	v.oneOf(e, "label_font_style", path, []string{"normal", "italic", "oblique"})
	v.positiveInt(e, "label_max_lines", path)
	for _, k := range []string{"label_font_family", "autocomplete"} {
		v.str(e, k, path)
	}
	if weight, ok := e["label_font_weight"]; ok {
		switch w := weight.(type) {
		case string:
			if w != "normal" && w != "bold" {
				n, err := strconv.Atoi(w)
				if err != nil || n < 100 || n > 900 || n%100 != 0 {
					v.errf(path+"/label_font_weight", "must be normal, bold, or a weight from 100 to 900")
				}
			}
		default:
			n, ok := asInt(w)
			if !ok || n < 100 || n > 900 || n%100 != 0 {
				v.errf(path+"/label_font_weight", "must be normal, bold, or a weight from 100 to 900")
			}
		}
	}
	v.matching(e, "label_color", path, colorPattern)
	for _, k := range []string{"max_length"} {
		if x, ok := e[k]; ok {
			if n, ok := asInt(x); !ok || n < 0 {
				v.errf(path+"/"+k, "must be a non-negative integer")
			}
		}
	}
	if value, ok := e["validation"]; ok {
		rules, ok := v.object(value, path+"/validation")
		if !ok {
			return
		}
		v.keys(rules, path+"/validation", nil, "min_length", "pattern", "min", "max", "step")
		if x, ok := rules["min_length"]; ok {
			if n, ok := asInt(x); !ok || n < 0 {
				v.errf(path+"/validation/min_length", "must be a non-negative integer")
			}
		}
		if pattern, ok := v.str(rules, "pattern", path+"/validation"); ok {
			if _, err := regexp.Compile(pattern); err != nil || regexp.MustCompile(`\(\?[a-zA-Z<]|\\[AzQEpP]`).MatchString(pattern) {
				v.errf(path+"/validation/pattern", "must be a valid common regular expression")
			}
		}
		for _, k := range []string{"min", "max", "step"} {
			if x, ok := rules[k]; ok {
				n, valid := number(x)
				if !valid || (k == "step" && n <= 0) {
					v.errf(path+"/validation/"+k, "must be a number (positive for step)")
				}
			}
		}

		if min, ok := number(rules["min"]); ok {
			if max, ok := number(rules["max"]); ok && min > max {
				v.errf(path+"/validation/max", "must not be less than min")
			}
		}
		if min, ok := asInt(rules["min_length"]); ok {
			if max, ok := asInt(e["max_length"]); ok && min > max {
				v.errf(path+"/validation/min_length", "must not exceed max_length")
			}
		}
	}
	if e["input_type"] == "number" {
		if _, ok := e["max_length"]; ok {
			v.errf(path+"/max_length", "is not supported for number")
		}
	}
}

// elementComplete checks a merged element instance: every attribute its code
// module requires is there, and the rules that span attributes hold.
func (v *validator) elementComplete(module string, merged map[string]any, path string) {
	code, _ := elementCode(module)
	required := code.required
	if module == "input" || module == "button" {
		required = slices.DeleteFunc(slices.Clone(required), func(key string) bool {
			return module == "input" && key == "data_binding" || module == "button" && key == "on_click"
		})
	}
	v.required(merged, path, required)
	v.chatComplete(module, merged, path)
	switch module {
	case "text":
		_, hasText := merged["text"]
		_, hasBinding := merged["data_binding"]
		if (hasText && hasBinding) || (!hasText && !hasBinding && !v.allowIncomplete) {
			v.errf(path, "a text element needs exactly one of %q or %q", "text", "data_binding")
		}
	case "menu":
		if initial, ok := merged["initial_item"].(string); ok && (!v.allowIncomplete || merged["items"] != nil) && !menuLeaves(merged["items"])[initial] {
			v.errf(path+"/initial_item", "initial_item %q is not a menu item that opens a page", initial)
		}
	}
}

func (v *validator) required(obj map[string]any, path string, keys []string) {
	if v.allowIncomplete {
		return
	}
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
	v.length(obj, key, "", lengthKind{auto: allowAuto, percent: true})
}

// lengthKind says which values besides a length (a non-negative number with an
// optional unit px, rem or em, px when left out) a length attribute accepts.
type lengthKind struct{ auto, elastic, percent bool }

// length checks a length attribute; a percentage is within [0, 100].
func (v *validator) length(obj map[string]any, key, path string, k lengthKind) {
	s, ok := v.str(obj, key, path)
	if !ok || (k.auto && s == "auto") || (k.elastic && s == "elastic") {
		return
	}
	m := pageLengthPattern.FindStringSubmatch(s)
	if m == nil || (!k.percent && m[3] == "%") {
		units := "px, rem, em"
		if k.percent {
			units += ", %"
		}
		var extra []string
		if k.elastic {
			extra = append(extra, "elastic")
		}
		if k.auto {
			extra = append(extra, "auto")
		}
		or := ""
		if len(extra) > 0 {
			or = " or " + strings.Join(extra, " or ")
		}
		v.errf(path+"/"+key, "%q must be a non-negative number with an optional unit (%s)%s", s, units, or)
		return
	}
	if n, _ := strconv.ParseFloat(m[1], 64); m[3] == "%" && n > 100 {
		v.errf(path+"/"+key, "%q must be within 0%% and 100%%", s)
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
	panels, _ := page["panels"].(map[string]any)
	for _, paneID := range slices.Sorted(maps.Keys(panels)) {
		panel, _ := panels[paneID].(map[string]any)
		els, _ := panel["elem_list"].([]any)
		for i, e := range els {
			m, _ := e.(map[string]any)
			path := "/panels/" + paneID + "/elem_list/" + strconv.Itoa(i)
			add := func(attr string, allowed, roots []string) {
				if val, ok := m[attr]; ok && !(attr == "on_click" && val == nil) {
					out = append(out, actionPos{val, path + "/" + attr, allowed, roots, false})
				}
			}
			add("on_click", triggerClasses, []string{RootInputDoc})
			add("source", sourceClasses, []string{RootInputDoc})
			add("on_row_click", triggerClasses, []string{RootInputDoc, RootRecord})
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

// actionInstance checks an action instance: its class has code and is allowed
// here, it holds a valid copy of its class, and the attributes it sets are
// defined by that code, valid, and complete against the copy. It returns the
// instance merged with its copy, or nil for static data or an invalid instance.
func (v *validator) actionInstance(pos actionPos) map[string]any {
	m, ok := v.object(pos.val, pos.path)
	if !ok || pos.isStatic() {
		return nil
	}
	cls, ok := v.str(m, "action", pos.path)
	if !ok {
		if _, present := m["action"]; !present {
			v.errf(pos.path, "missing attribute %q", "action")
		}
		return nil
	}
	if _, known := actionAttrs[cls]; !known {
		v.errf(pos.path+"/action", "action class %q has no code in this interpreter", cls)
		return nil
	}
	if !slices.Contains(pos.allowed, cls) {
		v.errf(pos.path+"/action", "a %s action is not allowed here (allowed: %s)", cls, strings.Join(pos.allowed, ", "))
		return nil
	}
	if _, present := m[copyKey]; !present {
		v.errf(pos.path, "missing attribute %q", copyKey)
	}
	v.noClassAttrs(m, pos.path)
	own := instanceAttrs(m, "action")
	v.action(cls, own, pos.path, "args")
	v.values(m, "args", pos.path, pos.roots...)
	row, ok := v.classCopy(KindAction, m, "action", pos.path)
	if !ok {
		return nil
	}
	before := len(v.issues)
	merged := v.againstCopy(KindAction, cls, row, own, pos.path)
	v.required(merged, pos.path, actionAttrs[cls].required)
	if len(v.issues) > before {
		return nil
	}
	return merged
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

// againstCopy checks an instance's own attributes against the class copy it
// holds and returns the merged instance. Every attribute whose default in the
// copy is null must be set by the instance. An element instance may set only
// the attributes its copy names, plus the size attributes every element has,
// and none that its copy fixes (devdoc 2026102601 §3.5.1). validators is like
// the sizes for the modules that take it (OpenSpec wpi-primitive-validators, D3).
func (v *validator) againstCopy(kind Kind, cls string, row, own map[string]any, path string) map[string]any {
	if kind == KindElement {
		for _, k := range slices.Sorted(maps.Keys(own)) {
			_, named := row[k]
			switch {
			case isFixed(row, k):
				v.errf(path+"/"+k, "attribute %q is fixed by element class %q; an instance cannot set it", k, cls)
			case !named && k == "validators" && !acceptsValidators(str(row["code_module_id"])):
				v.errf(path+"/"+k, "code module %q does not take validators; only %v elements do", str(row["code_module_id"]), validatorModules)
			case !named && str(row["code_module_id"]) == "chat-composer" && (k == "message_box_height" || k == "toolbar_height"):
				// These optional sizes also work with older embedded class copies.
			case !named && !slices.Contains(elementSizeAttrs, k) && k != "validators":
				v.errf(path+"/"+k, "element class %q has no attribute %q", cls, k)
			}
		}
	}
	_, required := classDefaults(kind, row)
	for _, k := range required {
		if kind == KindElement && ((str(row["code_module_id"]) == "input" && k == "data_binding") || (str(row["code_module_id"]) == "button" && k == "on_click")) {
			continue
		}
		if v.allowIncomplete {
			break
		}
		if _, set := own[k]; !set {
			v.errf(path, "attribute %q has no default in this instance's copy of %s class %q, so the instance must set it", k, kind, cls)
		}
	}
	return mergeInstance(kind, row, own)
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

// mergedElement is an element instance merged with its class copy, which it
// keeps for the checks that need the database.
type mergedElement struct {
	cls, path string
	body      map[string]any
	copy      map[string]any
}

// mergedAction is an action instance merged with its class copy.
type mergedAction struct {
	page, path, cls string
	body            map[string]any
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

func validatePageForSave(id string, body map[string]any) []Issue {
	return newValidatorMode(KindPage, id, body, true).issues
}

// surfaceColors accepts a fixed CSS color or separate light and dark colors.
func (v *validator) surfaceColors(obj map[string]any, path string) {
	for _, key := range []string{"background_color", "foreground_color"} {
		raw, exists := obj[key]
		if !exists || raw == nil {
			continue
		}
		if value, ok := raw.(string); ok {
			if value == "auto" || !colorPattern.MatchString(value) {
				v.errf(path+"/"+key, "invalid color %q", value)
			}
			continue
		}
		v.surfaceColorPair(raw, path+"/"+key)
	}
}

func (v *validator) surfaceColorPair(raw any, path string) {
	pair, ok := v.object(raw, path)
	if !ok {
		return
	}
	v.keys(pair, path, []string{"light", "dark"})
	for _, mode := range []string{"light", "dark"} {
		value, ok := pair[mode].(string)
		if !ok || value == "auto" || !colorPattern.MatchString(value) {
			v.errf(path+"/"+mode, "must be a CSS color")
		}
	}
}
