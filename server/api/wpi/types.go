// Package wpi is the backend of the Web Page Interpreter: descriptor storage
// and validation, the app bundle endpoint, page search, and the actions that
// run on the server (load, save, query, load-file).
// Design: KnowledgeStore devdoc 2026102601 and the OpenSpec changes wpi-pilot,
// wpi-classes-and-instances, wpi-pages, wpi-create-element-class,
// wpi-code-modules and wpi-pages-own-panes.
package wpi

import "encoding/json"

// Descriptor formats by kind (devdoc 2026102601 §4). Apps, pages and layouter
// classes are written in SchemaVersion (OpenSpec wpi-pages-own-panes: pages own
// their panes, and instances and layouter copies hold a copy of their class).
// Element class rows are written in ElementSchemaVersion, where each instance
// attribute is an object with a help tip, a default and whether instances may
// change it, and the class names its code module in code_module_id. Action class
// rows are written in ActionSchemaVersion, and validator class rows, which hold
// their constraints in a constraints object, in ValidatorSchemaVersion (OpenSpec
// wpi-primitive-validators, design D2). These are the only formats this
// interpreter accepts.
const (
	SchemaVersion          = "0.5"
	ElementSchemaVersion   = "0.4"
	ActionSchemaVersion    = "0.2"
	ValidatorSchemaVersion = "0.1"
)

// schemaVersionOf returns the descriptor format of a kind.
func schemaVersionOf(kind Kind) string {
	switch kind {
	case KindElement:
		return ElementSchemaVersion
	case KindAction:
		return ActionSchemaVersion
	case KindValidator:
		return ValidatorSchemaVersion
	}
	return SchemaVersion
}

// Kind is the kind of a stored descriptor: kb.wpi_descriptors.kind, or page
// for a row of kb.frontend_pages.
type Kind string

const (
	KindApp             Kind = "app"
	KindPage            Kind = "page"
	KindElement         Kind = "element"
	KindAction          Kind = "action"
	KindPageLayouter    Kind = "page-layouter"
	KindElementLayouter Kind = "element-layouter"
	KindValidator       Kind = "validator"
)

func validKind(k string) bool {
	switch Kind(k) {
	case KindApp, KindPage, KindElement, KindAction, KindPageLayouter, KindElementLayouter, KindValidator:
		return true
	}
	return false
}

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

// Bundle is an app with every page reachable from it, keyed by ID. Instances
// carry copies of their classes, so no class rows are sent.
type Bundle struct {
	App      json.RawMessage            `json:"app"`
	Pages    map[string]json.RawMessage `json:"pages"`
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

// Action is the typed view of a validated, merged load, save, query or
// load-file instance.
type Action struct {
	Class     string         `json:"action"`
	Table     string         `json:"table"`
	SQLStmtID string         `json:"sql_stmt_id"`
	Params    []any          `json:"params"`
	FileID    string         `json:"file_id"`
	Result    string         `json:"result"`
	Where     map[string]any `json:"where"`
	Fields    []FieldMap     `json:"fields"`
	OrderBy   []OrderBy      `json:"order_by"`
	Limit     *int           `json:"limit"`
	Mode      string         `json:"mode"`
	Key       string         `json:"key"`
}
