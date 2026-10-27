// Package wpi is the backend of the Web Page Interpreter: descriptor storage
// and validation, the app bundle endpoint, and the load/save actions.
// Design: KnowledgeStore devdoc 2026102601, openspec/changes/wpi-pilot and
// openspec/changes/wpi-classes-and-instances.
package wpi

import "encoding/json"

// SchemaVersion is the only descriptor format this interpreter accepts.
const SchemaVersion = "0.2"

// Kind is the kind of a stored descriptor (kb.wpi_descriptors.kind).
type Kind string

const (
	KindApp     Kind = "app"
	KindPage    Kind = "page"
	KindElement Kind = "element"
	KindAction  Kind = "action"
)

func validKind(k string) bool {
	switch Kind(k) {
	case KindApp, KindPage, KindElement, KindAction:
		return true
	}
	return false
}

// templatePanes lists the pane IDs each layout template provides.
var templatePanes = map[string][]string{
	"single-pane":                  {"main"},
	"top-left-middle-right-bottom": {"top", "left", "middle", "right", "bottom"},
}

var layouters = []string{"flow-layouter"}

// Descriptor is the latest active version of one stored descriptor.
type Descriptor struct {
	Kind    Kind
	ID      string
	Version int
	Raw     json.RawMessage
	Body    map[string]any
}

// Issue is one validation error or warning. Path is a JSON Pointer into the
// descriptor body ("" for the descriptor as a whole).
type Issue struct {
	Kind         Kind   `json:"kind"`
	DescriptorID string `json:"descriptor_id"`
	Path         string `json:"path"`
	Message      string `json:"message"`
}

// Bundle is an app with every page reachable from it and the element and
// action classes those pages use, keyed by ID.
type Bundle struct {
	App      json.RawMessage            `json:"app"`
	Pages    map[string]json.RawMessage `json:"pages"`
	Elements map[string]json.RawMessage `json:"elements"`
	Actions  map[string]json.RawMessage `json:"actions"`
	Warnings []Issue                    `json:"warnings"`
}

// FieldMap maps one input doc attribute to one table column.
type FieldMap struct {
	Attribute string `json:"attribute"`
	Column    string `json:"column"`
	Required  bool   `json:"required"`
	Rules     *Rules `json:"rules"`
}

// Rules are the server-side checks a save applies to one attribute.
type Rules struct {
	MaxLength *int   `json:"max_length"`
	Pattern   string `json:"pattern"`
	Format    string `json:"format"`
}

// OrderBy is one sort key of a list load.
type OrderBy struct {
	Column    string `json:"column"`
	Direction string `json:"direction"`
}

// Action is the typed view of a validated, merged load or save instance.
type Action struct {
	Class   string         `json:"action"`
	Table   string         `json:"table"`
	Result  string         `json:"result"`
	Where   map[string]any `json:"where"`
	Fields  []FieldMap     `json:"fields"`
	OrderBy []OrderBy      `json:"order_by"`
	Limit   *int           `json:"limit"`
	Mode    string         `json:"mode"`
	Key     string         `json:"key"`
}
