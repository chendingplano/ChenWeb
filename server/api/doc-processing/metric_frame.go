package docprocessing

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/width"
)

// Structured metric frame (openspec change metric-frame). A value is read inside a frame: the
// resources it uses (an agent and its strength, an apparatus and its grade), the method that
// applies or measures it, and its place in a procedure. condition / condition_en stay the
// condition facet: when, where, for whom or in what state the value holds.

type metricFrameSpec struct {
	Property   string `json:"property"`
	PropertyEn string `json:"property_en,omitempty"`
	Value      string `json:"value"`
	Unit       string `json:"unit,omitempty"`
}

type metricFrameResource struct {
	Role   string            `json:"role"`
	Name   string            `json:"name"`
	NameEn string            `json:"name_en,omitempty"`
	Spec   []metricFrameSpec `json:"spec,omitempty"`
}

type metricFrameQualifier struct {
	Label   string `json:"label"`
	Value   string `json:"value"`
	LabelEn string `json:"label_en,omitempty"`
	ValueEn string `json:"value_en,omitempty"`
}

type metricFrameProcedure struct {
	Step    string `json:"step,omitempty"`
	Stage   string `json:"stage,omitempty"`
	StageEn string `json:"stage_en,omitempty"`
	After   string `json:"after,omitempty"`
	AfterEn string `json:"after_en,omitempty"`
}

type metricFrame struct {
	Uses      []metricFrameResource  `json:"uses,omitempty"`
	Method    []metricFrameQualifier `json:"method,omitempty"`
	Procedure *metricFrameProcedure  `json:"procedure,omitempty"`
}

// metricFrameRoles is the closed list of resource roles (design D1).
var metricFrameRoles = []string{"agent", "material", "apparatus", "reagent", "medium", "sample", "device"}

func (f *metricFrame) isEmpty() bool {
	return f == nil || (len(f.Uses) == 0 && len(f.Method) == 0 && f.Procedure == nil)
}

// metricFrameFromValue decodes a frame from Pass 2 output, a row map or a scanned jsonb column.
// It returns nil when the value holds no facet. invalidRoles counts resources whose role is not
// in metricFrameRoles; they are kept with an empty role.
func metricFrameFromValue(v any) (frame *metricFrame, invalidRoles int) {
	var raw map[string]any
	switch x := v.(type) {
	case nil:
		return nil, 0
	case *metricFrame:
		if x.isEmpty() {
			return nil, 0
		}
		return x, 0
	case metricFrame:
		if x.isEmpty() {
			return nil, 0
		}
		return &x, 0
	case map[string]any:
		raw = x
	case []byte, json.RawMessage, string:
		var b []byte
		switch y := x.(type) {
		case []byte:
			b = y
		case json.RawMessage:
			b = y
		case string:
			b = []byte(y)
		}
		if json.Unmarshal(b, &raw) != nil {
			return nil, 0
		}
	default:
		return nil, 0
	}
	f := &metricFrame{}
	for _, item := range anySlice(raw["uses"]) {
		r, ok := item.(map[string]any)
		if !ok {
			continue
		}
		res := metricFrameResource{
			Role:   strings.ToLower(strings.TrimSpace(asString(r["role"]))),
			Name:   strings.TrimSpace(asString(r["name"])),
			NameEn: strings.TrimSpace(asString(r["name_en"])),
		}
		if res.Name == "" {
			continue
		}
		if !slices.Contains(metricFrameRoles, res.Role) {
			if res.Role != "" {
				invalidRoles++
			}
			res.Role = ""
		}
		for _, s := range anySlice(r["spec"]) {
			sm, ok := s.(map[string]any)
			if !ok {
				continue
			}
			spec := metricFrameSpec{
				Property:   strings.TrimSpace(asString(sm["property"])),
				PropertyEn: strings.TrimSpace(asString(sm["property_en"])),
				Value:      strings.TrimSpace(asString(sm["value"])),
				Unit:       strings.TrimSpace(asString(sm["unit"])),
			}
			if spec.Value != "" {
				res.Spec = append(res.Spec, spec)
			}
		}
		f.Uses = append(f.Uses, res)
	}
	for _, item := range anySlice(raw["method"]) {
		q, ok := item.(map[string]any)
		if !ok {
			continue
		}
		mq := metricFrameQualifier{
			Label:   strings.TrimSpace(asString(q["label"])),
			Value:   strings.TrimSpace(asString(q["value"])),
			LabelEn: strings.TrimSpace(asString(q["label_en"])),
			ValueEn: strings.TrimSpace(asString(q["value_en"])),
		}
		if mq.Value != "" {
			f.Method = append(f.Method, mq)
		}
	}
	if p, ok := raw["procedure"].(map[string]any); ok {
		proc := &metricFrameProcedure{
			Step:    strings.TrimSpace(asString(p["step"])),
			Stage:   strings.TrimSpace(asString(p["stage"])),
			StageEn: strings.TrimSpace(asString(p["stage_en"])),
			After:   strings.TrimSpace(asString(p["after"])),
			AfterEn: strings.TrimSpace(asString(p["after_en"])),
		}
		if *proc != (metricFrameProcedure{}) {
			f.Procedure = proc
		}
	}
	if f.isEmpty() {
		return nil, invalidRoles
	}
	return f, invalidRoles
}

func anySlice(v any) []any {
	s, _ := v.([]any)
	return s
}

// metricRowFrame returns the row's frame, or nil.
func metricRowFrame(m map[string]any) *metricFrame {
	f, _ := metricFrameFromValue(m["frame"])
	return f
}

// setMetricRowFrame stores f on the row, or removes the key when f is empty.
func setMetricRowFrame(m map[string]any, f *metricFrame) {
	if f.isEmpty() {
		delete(m, "frame")
		return
	}
	m["frame"] = f
}

// cloneMetricFrame returns a deep copy, so one row's edits never reach another row that was
// cloned from it.
func cloneMetricFrame(f *metricFrame) *metricFrame {
	if f == nil {
		return &metricFrame{}
	}
	out := &metricFrame{Method: slices.Clone(f.Method)}
	for _, r := range f.Uses {
		r.Spec = slices.Clone(r.Spec)
		out.Uses = append(out.Uses, r)
	}
	if f.Procedure != nil {
		p := *f.Procedure
		out.Procedure = &p
	}
	return out
}

// metricFrameSQLValue binds kb.metrics.frame: JSON, or NULL when the row has no frame.
func metricFrameSQLValue(m map[string]any) any {
	f := metricRowFrame(m)
	if f == nil {
		return nil
	}
	b, err := json.Marshal(f)
	if err != nil {
		return nil
	}
	return string(b)
}

// metricFrameKeyPart folds width, case and whitespace for identity comparison.
func metricFrameKeyPart(s string) string {
	return strings.ToLower(metricConditionCompact(s))
}

// metricFrameIdentityKey is the uses signature and method of a row (design D4): the sorted list of
// (role, name, spec) and of method values. procedure is not part of identity: the same metric
// restated at another step is an occurrence.
func metricFrameIdentityKey(m map[string]any) string {
	f := metricRowFrame(m)
	if f == nil {
		return ""
	}
	var uses []string
	for _, r := range f.Uses {
		specs := make([]string, 0, len(r.Spec))
		for _, s := range r.Spec {
			specs = append(specs, metricFrameKeyPart(s.Property)+"="+metricFrameKeyPart(s.Value)+metricFrameKeyPart(s.Unit))
		}
		slices.Sort(specs)
		uses = append(uses, r.Role+":"+metricFrameKeyPart(r.Name)+"{"+strings.Join(specs, ",")+"}")
	}
	slices.Sort(uses)
	var methods []string
	for _, q := range f.Method {
		methods = append(methods, metricFrameKeyPart(q.Value))
	}
	slices.Sort(methods)
	if len(uses) == 0 && len(methods) == 0 {
		return ""
	}
	return "uses=" + strings.Join(uses, ";") + "|method=" + strings.Join(methods, ";")
}

// metricFramesEqual reports whether two rows have the same uses signature and method.
func metricFramesEqual(a, b map[string]any) bool {
	return metricFrameIdentityKey(a) == metricFrameIdentityKey(b)
}

// Facets a legacy condition label moves to (design D5).
const (
	metricFacetUses  = "uses"
	metricFacetMeth  = "method"
	metricFacetStage = "stage"
	metricFacetSpec  = "spec"
)

type metricLabelFacet struct {
	facet string
	role  string // for metricFacetUses
}

// metricConditionLabelFacets maps the qualifier labels found in kb.metrics.condition (2026-10-18)
// and their usual synonyms to a facet. A label not listed stays in condition. 器材类型 (游标式,
// 指针式) is a variant of the specified object, so it is a condition and not listed here.
var metricConditionLabelFacets = map[string]metricLabelFacet{
	"试验装置": {metricFacetUses, "apparatus"}, "试验设备": {metricFacetUses, "apparatus"},
	"检验装置": {metricFacetUses, "apparatus"}, "检验设备": {metricFacetUses, "apparatus"},
	"测量仪器": {metricFacetUses, "apparatus"}, "仪器": {metricFacetUses, "apparatus"},
	"试验介质": {metricFacetUses, "medium"}, "介质": {metricFacetUses, "medium"},
	"溶媒": {metricFacetUses, "medium"}, "稀释液": {metricFacetUses, "medium"},
	"试剂": {metricFacetUses, "reagent"}, "试样": {metricFacetUses, "sample"}, "样品": {metricFacetUses, "sample"},
	"材料": {metricFacetUses, "material"}, "药物": {metricFacetUses, "agent"}, "药品": {metricFacetUses, "agent"}, "drug": {metricFacetUses, "agent"},
	"试验方法": {facet: metricFacetMeth}, "检验方法": {facet: metricFacetMeth}, "测量方法": {facet: metricFacetMeth},
	"测定方法": {facet: metricFacetMeth}, "试验方式": {facet: metricFacetMeth}, "给药途径": {facet: metricFacetMeth},
	"给药方式": {facet: metricFacetMeth}, "给氧方式": {facet: metricFacetMeth}, "操作": {facet: metricFacetMeth},
	"评估依据": {facet: metricFacetMeth},
	"治疗阶段": {facet: metricFacetStage}, "用药时机": {facet: metricFacetStage},
	"药物浓度":      {facet: metricFacetSpec},
	"apparatus": {metricFacetUses, "apparatus"}, "test apparatus": {metricFacetUses, "apparatus"},
	"equipment": {metricFacetUses, "apparatus"}, "medium": {metricFacetUses, "medium"},
	"test medium": {metricFacetUses, "medium"}, "solvent": {metricFacetUses, "medium"},
	"diluent": {metricFacetUses, "medium"}, "reagent": {metricFacetUses, "reagent"},
	"sample": {metricFacetUses, "sample"},
	"method": {facet: metricFacetMeth}, "test method": {facet: metricFacetMeth}, "route": {facet: metricFacetMeth},
	"administration route": {facet: metricFacetMeth}, "route of administration": {facet: metricFacetMeth},
	"oxygen delivery": {facet: metricFacetMeth}, "operation": {facet: metricFacetMeth},
	"assessment basis": {facet: metricFacetMeth},
	"treatment stage":  {facet: metricFacetStage}, "stage": {facet: metricFacetStage}, "timing": {facet: metricFacetStage},
	"drug concentration": {facet: metricFacetSpec},
}

// metricConditionLabelEnRe matches the label of an English qualifier, which is longer than a
// Chinese one ("Test apparatus: ", "Route of administration: ").
var metricConditionLabelEnRe = regexp.MustCompile(`^[^：:]{1,40}[：:]`)

// metricConditionLabel returns a qualifier's label (without its colon) and value.
func metricConditionLabel(q string) (label, value string) {
	return metricConditionLabelWith(metricConditionLabelRe, q)
}

// metricConditionLabelEn is metricConditionLabel for condition_en.
func metricConditionLabelEn(q string) (label, value string) {
	return metricConditionLabelWith(metricConditionLabelEnRe, q)
}

func metricConditionLabelWith(re *regexp.Regexp, q string) (label, value string) {
	q = strings.TrimSpace(q)
	l := re.FindString(q)
	label = strings.TrimSpace(strings.TrimRight(width.Fold.String(l), ":"))
	return label, strings.TrimSpace(strings.TrimPrefix(q, l))
}

var (
	// "50％葡萄糖", "0.1 mol/L 盐酸溶液": a leading strength.
	metricResourceLeadingStrengthRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(%|％|mol/L|g/L|mg/mL|mg/ml)\s*(\S.*)$`)
	// "肾上腺素（1:1000）": a trailing dilution.
	metricResourceTrailingRatioRe = regexp.MustCompile(`^(.+?)\s*[（(]\s*(\d+\s*[:：]\s*\d+)\s*[)）]$`)
	// "5 等量块": an apparatus grade.
	metricResourceGradeRe = regexp.MustCompile(`^(\d+)\s*(等|级)\s*(\S.*)$`)
	// "5%", "1:1000", "0.1 mol/L": a spec value with an optional unit.
	metricSpecValueRe = regexp.MustCompile(`^(\d+(?:\.\d+)?(?:\s*[:：]\s*\d+)?)\s*(%|％|mol/L|g/L|mg/mL|mg/ml)?$`)
)

// parseMetricResource splits a resource's strength or grade off its name: "50％葡萄糖注射液" →
// 葡萄糖注射液 {浓度 50 %}; "肾上腺素（1:1000）" → 肾上腺素 {浓度 1:1000}; "5 等量块" → 量块 {等级 5 等}.
func parseMetricResource(role, value string) metricFrameResource {
	value = strings.TrimSpace(value)
	if m := metricResourceLeadingStrengthRe.FindStringSubmatch(value); m != nil {
		return metricFrameResource{Role: role, Name: strings.TrimSpace(m[3]),
			Spec: []metricFrameSpec{{Property: "浓度", PropertyEn: "concentration", Value: m[1], Unit: width.Fold.String(m[2])}}}
	}
	if m := metricResourceTrailingRatioRe.FindStringSubmatch(value); m != nil {
		return metricFrameResource{Role: role, Name: strings.TrimSpace(m[1]),
			Spec: []metricFrameSpec{{Property: "浓度", PropertyEn: "concentration", Value: strings.ReplaceAll(width.Fold.String(m[2]), " ", "")}}}
	}
	if role == "apparatus" {
		if m := metricResourceGradeRe.FindStringSubmatch(value); m != nil {
			return metricFrameResource{Role: role, Name: strings.TrimSpace(m[3]),
				Spec: []metricFrameSpec{{Property: "等级", PropertyEn: "grade", Value: m[1] + " " + m[2]}}}
		}
	}
	return metricFrameResource{Role: role, Name: value}
}

// parseMetricSpecValue reads "1:1000" or "5%" into a value and unit; ok is false for free text.
func parseMetricSpecValue(value string) (string, string, bool) {
	m := metricSpecValueRe.FindStringSubmatch(strings.TrimSpace(width.Fold.String(value)))
	if m == nil {
		return "", "", false
	}
	return strings.ReplaceAll(m[1], " ", ""), m[2], true
}

// metricFrameHasResource reports whether f already uses a resource of the role whose name
// contains, or is contained in, name.
func metricFrameHasResource(f *metricFrame, role, name string) bool {
	n := metricFrameKeyPart(name)
	for _, r := range f.Uses {
		rn := metricFrameKeyPart(r.Name)
		if r.Role == role && rn != "" && (strings.Contains(rn, n) || strings.Contains(n, rn)) {
			return true
		}
	}
	return false
}

// metricFrameHasMethod reports whether f already has a method whose value overlaps value.
func metricFrameHasMethod(f *metricFrame, value string) bool {
	v := metricFrameKeyPart(value)
	for _, q := range f.Method {
		w := metricFrameKeyPart(q.Value)
		if w != "" && (strings.Contains(w, v) || strings.Contains(v, w)) {
			return true
		}
	}
	return false
}

// metricLegacySplitResult reports what splitLegacyCondition did to one row.
type metricLegacySplitResult struct {
	Moved           map[string]int // label → qualifiers moved out of condition
	UnresolvedSpecs int            // 药物浓度 qualifiers kept because the row has no single agent
}

// splitLegacyCondition moves the qualifiers of condition / condition_en whose label is in
// metricConditionLabelFacets into the row's frame (design D5). condition_en follows by position
// only when it has as many qualifiers as condition; otherwise it is left unchanged. A 药物浓度
// qualifier becomes the spec of the row's single agent (or reagent or medium) and stays in
// condition when there is none or more than one. Unknown labels are never moved.
func splitLegacyCondition(m map[string]any) metricLegacySplitResult {
	res := metricLegacySplitResult{Moved: map[string]int{}}
	cond := strings.TrimSpace(asString(m["condition"]))
	if cond == "" {
		return res
	}
	parts := compactNonEmptyStrings(strings.Split(strings.ReplaceAll(cond, ";", "；"), "；"))
	enParts := compactNonEmptyStrings(strings.Split(asString(m["condition_en"]), ";"))
	aligned := len(enParts) == len(parts)
	frame := cloneMetricFrame(metricRowFrame(m))
	keep := make([]bool, len(parts))
	var specIdx []int
	for i, q := range parts {
		keep[i] = true
		label, value := metricConditionLabel(q)
		lf, ok := metricConditionLabelFacets[strings.ToLower(label)]
		if !ok || value == "" {
			continue
		}
		labelEn, valueEn := "", ""
		if aligned {
			labelEn, valueEn = metricConditionLabelEn(enParts[i])
		}
		switch lf.facet {
		case metricFacetUses:
			r := parseMetricResource(lf.role, value)
			if valueEn != "" {
				r.NameEn = parseMetricResource(lf.role, valueEn).Name
			}
			if !metricFrameHasResource(frame, r.Role, r.Name) {
				frame.Uses = append(frame.Uses, r)
			}
		case metricFacetMeth:
			if !metricFrameHasMethod(frame, value) {
				frame.Method = append(frame.Method, metricFrameQualifier{Label: label, Value: value, LabelEn: labelEn, ValueEn: valueEn})
			}
		case metricFacetStage:
			if frame.Procedure == nil {
				frame.Procedure = &metricFrameProcedure{}
			}
			frame.Procedure.Stage = strings.TrimPrefix(frame.Procedure.Stage+"；"+value, "；")
			if valueEn != "" {
				frame.Procedure.StageEn = strings.TrimPrefix(frame.Procedure.StageEn+"; "+valueEn, "; ")
			}
		case metricFacetSpec:
			specIdx = append(specIdx, i) // after the resources of this row are known
			continue
		}
		keep[i] = false
		res.Moved[label]++
	}
	for _, i := range specIdx {
		label, value := metricConditionLabel(parts[i])
		var hosts []int
		for j, r := range frame.Uses {
			if r.Role == "agent" || r.Role == "reagent" || r.Role == "medium" {
				hosts = append(hosts, j)
			}
		}
		v, unit, ok := parseMetricSpecValue(value)
		if len(hosts) != 1 || !ok {
			res.UnresolvedSpecs++
			continue
		}
		frame.Uses[hosts[0]].Spec = append(frame.Uses[hosts[0]].Spec,
			metricFrameSpec{Property: "浓度", PropertyEn: "concentration", Value: v, Unit: unit})
		keep[i] = false
		res.Moved[label]++
	}
	if len(res.Moved) == 0 {
		return res
	}
	var kept, keptEn []string
	for i, q := range parts {
		if keep[i] {
			kept = append(kept, strings.TrimSpace(q))
			if aligned {
				keptEn = append(keptEn, strings.TrimSpace(enParts[i]))
			}
		}
	}
	m["condition"] = strings.Join(kept, "；")
	if aligned {
		m["condition_en"] = strings.Join(keptEn, "; ")
	}
	setMetricRowFrame(m, frame)
	return res
}

// metricProvisionIDSQLValue binds kb.metrics.provision_id: a positive id, or NULL.
func metricProvisionIDSQLValue(v any) any {
	switch x := v.(type) {
	case int64:
		if x > 0 {
			return x
		}
	case int:
		if x > 0 {
			return int64(x)
		}
	case float64:
		if x > 0 {
			return int64(x)
		}
	}
	return nil
}

// metricFrameInvalidRolesKey carries, from normalizeMetricList to applyMetricFrames, the number of
// Pass 2 resources whose role was not in metricFrameRoles. It never reaches the database.
const metricFrameInvalidRolesKey = "_frame_invalid_roles"

// applyMetricFrames moves legacy qualifiers out of condition into the frame on Pass 2 rows, as a
// safety net for model output that still writes an apparatus, method or stage as a condition
// (design D5), and logs what it moved.
func (p *MetricsProcessor) applyMetricFrames(recordID int64, set string, metrics []map[string]any) {
	moved := map[string]int{}
	unresolved, invalid := 0, 0
	for _, m := range metrics {
		if n, ok := m[metricFrameInvalidRolesKey].(int); ok {
			invalid += n
			delete(m, metricFrameInvalidRolesKey)
		}
		res := splitLegacyCondition(m)
		for label, n := range res.Moved {
			moved[label] += n
		}
		unresolved += res.UnresolvedSpecs
	}
	if len(moved) == 0 && unresolved == 0 && invalid == 0 {
		return
	}
	p.Logger.Info("pass2: moved legacy condition qualifiers into the metric frame",
		"record_id", recordID, "rows", set, "moved", moved,
		"unresolved_specs", unresolved, "invalid_resource_roles", invalid)
}

// metricFrameItem is one resource or method of a frame, for the inheritance steps.
type metricFrameItem struct {
	resource *metricFrameResource
	method   *metricFrameQualifier
}

func (it metricFrameItem) value() string {
	switch {
	case it.resource != nil:
		return it.resource.Name
	case it.method != nil:
		return it.method.Value
	}
	return ""
}

// metricFrameItemOf returns the row's first frame item of a kind: "uses:<role>" or
// "method:<label>".
func metricFrameItemOf(m map[string]any, kind string) (metricFrameItem, bool) {
	f := metricRowFrame(m)
	if f == nil {
		return metricFrameItem{}, false
	}
	facet, key, _ := strings.Cut(kind, ":")
	switch facet {
	case "uses":
		for i := range f.Uses {
			if f.Uses[i].Role == key {
				r := f.Uses[i]
				return metricFrameItem{resource: &r}, true
			}
		}
	case "method":
		for i := range f.Method {
			if f.Method[i].Label == key {
				q := f.Method[i]
				return metricFrameItem{method: &q}, true
			}
		}
	}
	return metricFrameItem{}, false
}

// metricFrameOverlaps reports whether the row already names value in its frame or condition: the
// same apparatus under another facet (a 脉冲信号发生器 resource vs a 脉冲信号发生器法 method).
func metricFrameOverlaps(m map[string]any, value string) bool {
	if metricConditionOverlaps(m, "x："+value) {
		return true
	}
	f := metricRowFrame(m)
	if f == nil {
		return false
	}
	for _, r := range f.Uses {
		if metricFrameHasResource(&metricFrame{Uses: []metricFrameResource{r}}, r.Role, value) {
			return true
		}
	}
	return metricFrameHasMethod(f, value)
}

// addMetricFrameItem appends a copy of item to the row's frame.
func addMetricFrameItem(m map[string]any, item metricFrameItem) {
	f := cloneMetricFrame(metricRowFrame(m))
	if item.resource != nil {
		r := *item.resource
		r.Spec = slices.Clone(r.Spec)
		f.Uses = append(f.Uses, r)
	}
	if item.method != nil {
		f.Method = append(f.Method, *item.method)
	}
	setMetricRowFrame(m, f)
}

// inheritServedFrame copies to row the resources and methods of served that it lacks. It reports
// whether the row changed.
func inheritServedFrame(row, served map[string]any) bool {
	sf := metricRowFrame(served)
	if sf == nil {
		return false
	}
	changed := false
	for i := range sf.Uses {
		r := sf.Uses[i]
		if f := metricRowFrame(row); f != nil && metricFrameHasResource(f, r.Role, r.Name) {
			continue
		}
		addMetricFrameItem(row, metricFrameItem{resource: &r})
		changed = true
	}
	for i := range sf.Method {
		q := sf.Method[i]
		if f := metricRowFrame(row); f != nil && metricFrameHasMethod(f, q.Value) {
			continue
		}
		addMetricFrameItem(row, metricFrameItem{method: &q})
		changed = true
	}
	return changed
}

// metricDropReasonResourceSpec marks a row that only states the spec of a resource another row
// uses (design D6).
const metricDropReasonResourceSpec = "resource_spec"

var (
	metricRatioValueRe  = regexp.MustCompile(`^\d+(?:\.\d+)?:\d+(?:\.\d+)?$`)
	metricNumberValueRe = regexp.MustCompile(`^\d+(?:\.\d+)?$`)
)

// metricResourceSpecUnits are the units of a strength or concentration.
var metricResourceSpecUnits = []string{"%", "mol/l", "mmol/l", "g/l", "mg/ml", "g/ml", "ratio"}

// metricSpecNorm folds a spec value or unit for comparison.
func metricSpecNorm(s string) string {
	return strings.ToLower(strings.ReplaceAll(metricConditionCompact(s), "：", ":"))
}

// metricResourceSpecValue reports whether the row's value reads as a strength, concentration or
// dilution: "1:1000", or a number in %, mol/L, g/L or mg/mL.
func metricResourceSpecValue(m map[string]any) (value, unit string, ok bool) {
	value = metricSpecNorm(asString(m["metric_value"]))
	unit = metricSpecNorm(metricRowUnit(m))
	if metricRatioValueRe.MatchString(value) {
		return value, unit, true
	}
	return value, unit, metricNumberValueRe.MatchString(value) && slices.Contains(metricResourceSpecUnits, unit)
}

// metricIdentifyingSpecProperties are the spec properties that only name which product is used.
// An accuracy or error limit the model wrote as a spec is a requirement on the resource, so it
// never makes a host for dropResourceSpecRows.
var metricIdentifyingSpecProperties = []string{"浓度", "含量", "稀释度", "稀释比", "规格", "等级", "级别", "型号",
	"concentration", "strength", "dilution", "grade", "class", "model"}

func metricIdentifyingSpec(s metricFrameSpec) bool {
	p := metricSpecNorm(firstNonEmptyTrimmed(s.Property, s.PropertyEn))
	pe := metricSpecNorm(s.PropertyEn)
	for _, k := range metricIdentifyingSpecProperties {
		if strings.Contains(p, k) || (pe != "" && strings.Contains(pe, k)) {
			return true
		}
	}
	return false
}

// metricFrameHasSpec reports whether the row uses a resource whose identifying spec has the value
// (and the unit, when both give one).
func metricFrameHasSpec(m map[string]any, value, unit string) bool {
	f := metricRowFrame(m)
	if f == nil {
		return false
	}
	for _, r := range f.Uses {
		for _, s := range r.Spec {
			if !metricIdentifyingSpec(s) {
				continue
			}
			su := metricSpecNorm(s.Unit)
			if metricSpecNorm(s.Value) == value && (su == "" || unit == "" || unit == "ratio" || su == unit) {
				return true
			}
		}
	}
	return false
}

// dropResourceSpecRows sets aside a row whose value is a strength, concentration or dilution that
// another row of an overlapping source line already carries as the spec of a resource it uses:
// "葡萄糖浓度 50 %" next to 葡萄糖静推剂量 40 ml using 葡萄糖注射液 {浓度 50 %} (364 A.2.4). The prompts do
// the main work; this guard only catches rows that slip through, so it needs the host row. A row
// that is the specified object's own property (a product standard's 含量) has no host and is kept.
// The set-aside row names its host by candidate_id and metric_name: metric ids are assigned later.
func dropResourceSpecRows(metrics []map[string]any) (kept []map[string]any, dropped []droppedMetricRow) {
	for i, m := range metrics {
		value, unit, ok := metricResourceSpecValue(m)
		var host map[string]any
		if ok && !metricFrameHasSpec(m, value, unit) {
			for j, o := range metrics {
				if j != i && metricLineSpansOverlap(m, o) && metricFrameHasSpec(o, value, unit) {
					host = o
					break
				}
			}
		}
		if host == nil {
			kept = append(kept, m)
			continue
		}
		row := cloneMetricMap(m)
		row["resource_spec_host"] = map[string]any{
			"candidate_id": asString(host["candidate_id"]),
			"metric_name":  asString(host["metric_name"]),
		}
		dropped = append(dropped, newDroppedMetricRow(row, metricDropStageEnrich, metricDropReasonResourceSpec))
	}
	return kept, dropped
}

// MetricProvisionLinker sets kb.metrics.provision_id from the procedure facet (spec
// metric-provision-link). MetricsSQLStore implements it; a store that does not is skipped.
type MetricProvisionLinker interface {
	LinkMetricProvisions(ctx context.Context, inputRecordID int64) (int64, error)
}

func (s *ResolvingMetricsStore) LinkMetricProvisions(ctx context.Context, inputRecordID int64) (int64, error) {
	inner, ok := s.Inner.(MetricProvisionLinker)
	if !ok {
		return 0, nil
	}
	return inner.LinkMetricProvisions(ctx, inputRecordID)
}

// LinkMetricProvisions links each unlinked metric of the record that has a procedure step to the
// one provision of the same record whose source lines contain all of the metric's lines. A metric
// covered by no provision, or by several, stays unlinked.
func (s MetricsSQLStore) LinkMetricProvisions(ctx context.Context, inputRecordID int64) (int64, error) {
	const q = `
WITH prov AS (
	SELECT p.id, range_agg(int8range(split_part(s, ':', 1)::int8,
		COALESCE(NULLIF(split_part(s, ':', 2), ''), split_part(s, ':', 1))::int8, '[]')) AS lines
	FROM kb.provisions p, jsonb_array_elements_text(p.source_line_spans) s
	WHERE p.input_record_id = $1 AND jsonb_typeof(p.source_line_spans) = 'array' AND s ~ '^\d+(:\d+)?$'
	GROUP BY p.id
), cand AS (
	SELECT m.id AS mid, pr.id AS pid, COUNT(*) OVER (PARTITION BY m.id) AS n
	FROM kb.metrics m JOIN prov pr ON m.line_range <@ pr.lines
	WHERE m.input_record_id = $1 AND m.ref_mention_id IS NULL AND m.provision_id IS NULL AND m.line_range IS NOT NULL
	  AND COALESCE(m.frame->'procedure'->>'step', '') <> ''
)
UPDATE kb.metrics m SET provision_id = c.pid FROM cand c WHERE m.id = c.mid AND c.n = 1`
	res, err := s.DB.ExecContext(ctx, q, inputRecordID)
	if err != nil {
		return 0, fmt.Errorf("(MID_26101801) link metric provisions for record_id=%d: %w", inputRecordID, err)
	}
	return res.RowsAffected()
}

// linkMetricProvisions runs the store's provision linker after a save. A failure is logged, not
// returned: the link is optional.
func (p *MetricsProcessor) linkMetricProvisions(ctx context.Context, recordID int64) {
	linker, ok := p.Store.(MetricProvisionLinker)
	if !ok {
		return
	}
	n, err := linker.LinkMetricProvisions(ctx, recordID)
	if err != nil {
		p.Logger.Warn("failed to link metrics to provisions", "record_id", recordID, "error", err)
		return
	}
	if n > 0 {
		p.Logger.Info("linked metrics to provisions by procedure step", "record_id", recordID, "rows", n)
	}
}
