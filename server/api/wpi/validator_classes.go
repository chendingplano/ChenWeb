package wpi

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/wpi/validators"
)

// Validator classes and instances (OpenSpec wpi-primitive-validators; KnowledgeStore
// doc-repo/code_modules/validator-primitive.md). A validator class is a row of
// kind validator that names its code module in code_module_id and lists the
// constraints instances may set; an instance lives in the validators list of the
// element that uses it and holds a whole copy of its class, like every instance.

// validatorModules are the element code modules whose instances may set
// validators, whether or not their class copy names it (design D3).
var validatorModules = []string{"input", "select", "value-editor"}

func acceptsValidators(module string) bool { return slices.Contains(validatorModules, module) }

// The attributes of a validator class row besides its constraints.
var (
	validatorClassRequired = []string{"descriptor_id", "schema_version", "code_module_id", "category", "description", "constraints"}
	validatorClassOptional = []string{"implement_notes", "value_types", "empty_values", "empty_constraint", "execution", "runs_in", "error_codes"}
)

// validatorClass checks a validator class row (format 0.1, design D2): it names
// a validator module that has code, and every fact it states about that code
// agrees with the code.
func (v *validator) validatorClass(b map[string]any) {
	v.keys(b, "", validatorClassRequired, validatorClassOptional...)
	v.text(b, "description", "")
	v.text(b, "implement_notes", "")
	v.oneOf(b, "category", "", validators.Categories)
	module, _ := b["code_module_id"].(string)
	spec, known := validators.Modules[module]
	if !known {
		if _, present := b["code_module_id"]; present {
			v.errf("/code_module_id", "validator code module %q has no code in this interpreter", module)
		}
		return
	}
	facts := map[string]any{"value_types": spec.ValueTypes, "empty_values": spec.EmptyValues, "execution": spec.Execution, "runs_in": spec.RunsIn}
	if spec.EmptyConstraint != "" {
		facts["empty_constraint"] = spec.EmptyConstraint
	}
	for _, k := range slices.Sorted(maps.Keys(facts)) {
		if got, present := b[k]; present && !sameJSON(got, facts[k]) {
			v.errf("/"+k, "must be %v, as code module %q states", facts[k], module)
		}
	}
	if _, present := b["empty_constraint"]; present && spec.EmptyConstraint == "" {
		v.errf("/empty_constraint", "code module %q has no empty constraint", module)
	}
	if val, present := b["error_codes"]; present {
		if codes, ok := v.object(val, "/error_codes"); ok {
			for _, code := range slices.Sorted(maps.Keys(codes)) {
				p := "/error_codes/" + pointerEscape(code)
				params, known := spec.ErrorCodes[code]
				if !known {
					v.errf(p, "code module %q reports no error %q", module, code)
					continue
				}
				entry, ok := v.object(codes[code], p)
				if !ok {
					continue
				}
				v.keys(entry, p, nil, "params")
				if got, present := entry["params"]; present && !sameJSON(got, params) {
					v.errf(p+"/params", "must be %v, as code module %q reports them", params, module)
				}
			}
		}
	}
	defs, ok := v.object(b["constraints"], "/constraints")
	if !ok {
		return
	}
	defaults := map[string]any{}
	for _, name := range slices.Sorted(maps.Keys(defs)) {
		p := "/constraints/" + pointerEscape(name)
		c, known := spec.Constraint(name)
		if !known {
			v.errf(p, "validator code %q has no constraint %q", module, name)
			continue
		}
		def, ok := v.object(defs[name], p)
		if !ok {
			continue
		}
		v.keys(def, p, nil, "type", "choices", "help_tip", "default_value", "instance_modifiable", "value_editor")
		if got, present := def["type"]; present && got != c.Type {
			v.errf(p+"/type", "must be %q, as code module %q defines it", c.Type, module)
		}
		if got, present := def["choices"]; present && !sameJSON(got, c.Choices) {
			v.errf(p+"/choices", "must be %v, as code module %q defines them", c.Choices, module)
		}
		v.text(def, "help_tip", p)
		v.boolean(def, "instance_modifiable", p)
		v.valueEditor(def, p)
		dv, hasDefault := def["default_value"]
		if def["instance_modifiable"] == false && (!hasDefault || dv == nil) {
			v.errf(p+"/default_value", "a constraint fixed by the class needs a default_value")
		}
		if hasDefault && dv != nil {
			if msg := validators.CheckValue(c, dv); msg != "" {
				v.errf(p+"/default_value", "%s", msg)
			} else {
				defaults[name] = dv
			}
		}
	}
	for _, pr := range validators.CheckConstraints(module, defaults) {
		v.errf("/constraints/"+pr.Constraint+"/default_value", "%s", pr.Message)
	}
}

// validatorList checks an element's validators: an array of validator instances
// {"validator": class, "class": copy, "constraints"?, "messages"?}, each checked
// against the copy it holds (design D3).
func (v *validator) validatorList(val any, path string) {
	list, ok := v.array(val, path, false)
	if !ok {
		return
	}
	for i, item := range list {
		p := fmt.Sprintf("%s/%d", path, i)
		m, ok := v.object(item, p)
		if !ok {
			continue
		}
		v.keys(m, p, []string{"validator", copyKey}, "constraints", "messages")
		row, ok := v.classCopy(KindValidator, m, "validator", p)
		if !ok {
			continue
		}
		v.validatorInstance(row, m, p)
	}
}

func (v *validator) validatorInstance(row, m map[string]any, path string) {
	cls, module := str(row["descriptor_id"]), str(row["code_module_id"])
	defs, _ := row["constraints"].(map[string]any)
	own := map[string]any{}
	if val, present := m["constraints"]; present {
		given, ok := v.object(val, path+"/constraints")
		if !ok {
			return
		}
		for _, name := range slices.Sorted(maps.Keys(given)) {
			def, named := defs[name].(map[string]any)
			switch {
			case !named:
				v.errf(path+"/constraints/"+pointerEscape(name), "validator class %q has no constraint %q", cls, name)
			case def["instance_modifiable"] == false:
				v.errf(path+"/constraints/"+pointerEscape(name), "constraint %q is fixed by validator class %q; an instance cannot set it", name, cls)
			default:
				own[name] = given[name]
			}
		}
	}
	for _, pr := range validators.CheckConstraints(module, own) {
		v.errf(path+"/constraints/"+pr.Constraint, "%s", pr.Message)
	}
	if !v.allowIncomplete {
		for _, name := range slices.Sorted(maps.Keys(defs)) {
			def, _ := defs[name].(map[string]any)
			if dv, has := def["default_value"]; has && dv == nil {
				if _, set := own[name]; !set {
					v.errf(path+"/constraints", "constraint %q has no default in this instance's copy of validator class %q, so the instance must set it", name, cls)
				}
			}
		}
	}
	if val, present := m["messages"]; present {
		messages, ok := v.object(val, path+"/messages")
		if !ok {
			return
		}
		codes, _ := row["error_codes"].(map[string]any)
		for _, code := range slices.Sorted(maps.Keys(messages)) {
			if _, known := codes[code]; !known {
				v.errf(path+"/messages/"+pointerEscape(code), "validator class %q reports no error %q", cls, code)
				continue
			}
			v.text(messages, code, path+"/messages")
		}
	}
}

// validatorConstraints returns the code module of a validator instance and the
// constraints its copy and the instance give, the instance's winning; the code
// adds its defaults when it runs.
func validatorConstraints(item map[string]any) (module string, constraints map[string]any) {
	row, _ := item[copyKey].(map[string]any)
	constraints = map[string]any{}
	defs, _ := row["constraints"].(map[string]any)
	for name, d := range defs {
		def, _ := d.(map[string]any)
		if dv, has := def["default_value"]; has && dv != nil {
			constraints[name] = dv
		}
	}
	own, _ := item["constraints"].(map[string]any)
	maps.Copy(constraints, own)
	for name, d := range defs {
		// A fixed constraint keeps the class's value whatever an instance sets.
		if def, _ := d.(map[string]any); def["instance_modifiable"] == false {
			constraints[name] = def["default_value"]
		}
	}
	return str(row["code_module_id"]), constraints
}

// runValidators checks value with every validator of an element (allOf) and
// returns their errors, each naming its validator's index in the list.
func runValidators(list []any, value any, path string) []validators.Error {
	var out []validators.Error
	for i, item := range list {
		m, _ := item.(map[string]any)
		module, constraints := validatorConstraints(m)
		for _, e := range validators.Run(module, constraints, value, path).Errors {
			e.Validator = i
			out = append(out, e)
		}
	}
	return out
}

// validatorPointer is the JSON Pointer of a bound attribute (§2.6.3).
func validatorPointer(segments []string) string {
	var b strings.Builder
	for _, s := range segments {
		b.WriteString("/" + pointerEscape(s))
	}
	return b.String()
}

func pointerEscape(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}

// sameJSON compares a decoded JSON value with a Go value by their JSON form.
func sameJSON(a, b any) bool {
	return reflect.DeepEqual(jsonForm(a), jsonForm(b))
}

func jsonForm(v any) any {
	switch x := v.(type) {
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case nil:
		return []any{}
	}
	return v
}
