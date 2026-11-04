// Package validators implements the WPI validators (KnowledgeStore
// doc-repo/validators/validator-requirements.md; OpenSpec wpi-primitive-validators):
// the code modules that check one value against configurable constraints and
// report standard errors. It has no database access. The TypeScript twin in
// web/src/lib/wpi/validators must give the same results; both read modules.json
// and pass validator-cases.json.
package validators

import (
	_ "embed"
	"encoding/json"
	"slices"
)

//go:embed modules.json
var modulesJSON []byte

// ConstraintSpec is one constraint a module accepts: its value type, the
// choices of a choice type, and the code default (HasDefault false: the
// constraint is not applied until set).
type ConstraintSpec struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	Choices    []string `json:"choices,omitempty"`
	Default    any      `json:"default,omitempty"`
	HasDefault bool     `json:"-"`
}

// ModuleSpec is what a validator code module accepts and reports. Constraints
// are in the order their checks run; ErrorCodes give each code's params.
type ModuleSpec struct {
	Category        string              `json:"category"`
	ValueTypes      []string            `json:"value_types"`
	EmptyValues     []string            `json:"empty_values"`
	EmptyConstraint string              `json:"empty_constraint"`
	Execution       string              `json:"execution"`
	RunsIn          string              `json:"runs_in"`
	Constraints     []ConstraintSpec    `json:"constraints"`
	ErrorCodes      map[string][]string `json:"error_codes"`
}

// Constraint returns the spec of one of the module's constraints.
func (m ModuleSpec) Constraint(name string) (ConstraintSpec, bool) {
	i := slices.IndexFunc(m.Constraints, func(c ConstraintSpec) bool { return c.Name == name })
	if i < 0 {
		return ConstraintSpec{}, false
	}
	return m.Constraints[i], true
}

// Categories are the validator categories of the requirements, §3.1–§3.11, in order.
var Categories []string

// Modules are the validator code modules, by code module ID.
var Modules map[string]ModuleSpec

func init() {
	var raw struct {
		Categories []string                   `json:"categories"`
		Modules    map[string]json.RawMessage `json:"modules"`
	}
	if err := json.Unmarshal(modulesJSON, &raw); err != nil {
		panic("validators: modules.json: " + err.Error())
	}
	Categories = raw.Categories
	Modules = map[string]ModuleSpec{}
	for id, body := range raw.Modules {
		var m ModuleSpec
		if err := json.Unmarshal(body, &m); err != nil {
			panic("validators: modules.json " + id + ": " + err.Error())
		}
		// Default is optional and may be false; read whether it is given.
		var shape struct {
			Constraints []map[string]json.RawMessage `json:"constraints"`
		}
		_ = json.Unmarshal(body, &shape)
		for i := range m.Constraints {
			_, m.Constraints[i].HasDefault = shape.Constraints[i]["default"]
		}
		Modules[id] = m
	}
}

// effective returns the module's code defaults overlaid with the given constraints.
func (m ModuleSpec) effective(given map[string]any) map[string]any {
	out := map[string]any{}
	for _, c := range m.Constraints {
		if c.HasDefault {
			out[c.Name] = c.Default
		}
	}
	for k, v := range given {
		out[k] = v
	}
	return out
}
