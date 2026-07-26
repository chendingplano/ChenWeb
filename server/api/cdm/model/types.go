// Package model defines the CDM v1.0 canonical document AST: the Go types,
// their JSON encoding, and the identifier rules from
// KnowledgeStore/doc-repo/specs/202607/2026072501-spec-canonical-doc-model.md.
//
// This package imports no database or ChenWeb-specific packages, so it can be
// promoted to shared/go later as a package move rather than a rewrite.
package model

import "time"

// SchemaVersion is the CDM schema version this package implements.
const SchemaVersion = "1.0"

// Document is the root canonical document (spec §1, §5.2).
type Document struct {
	Key            string   `json:"document_key"`
	Title          string   `json:"title"`
	Language       string   `json:"language"`
	SchemaVersion  string   `json:"schema_version"`
	ContentVersion int64    `json:"content_version"`
	Metadata       Metadata `json:"metadata"`
	Blocks         []Block  `json:"blocks"`
}

// Metadata is document-level metadata (spec §4.1).
type Metadata struct {
	DocType       string    `json:"doc_type,omitempty"`
	RenderingType string    `json:"rendering_type,omitempty"`
	Authors       []string  `json:"authors,omitempty"`
	Version       string    `json:"version,omitempty"`
	CreateTime    time.Time `json:"create_time,omitzero"`
	ModifyTime    time.Time `json:"modify_time,omitzero"`
}

// Block is the single struct for every block type. Which fields are
// populated is determined by Type; see the content-model invariants in
// spec §1.2 and Validate in this package.
type Block struct {
	ID   string `json:"id"`
	Type string `json:"type"`

	// Role carries callout severity only (spec §3): note, tip, important,
	// warning, caution.
	Role string `json:"role,omitempty"`

	// Level is the heading level, 1-6 (heading only).
	Level int `json:"level,omitempty"`

	// Title is the callout title (callout only).
	Title string `json:"title,omitempty"`

	// Term is the term being defined (definition only).
	Term string `json:"term,omitempty"`

	// Text is verbatim source text (code only).
	Text string `json:"text,omitempty"`
	// Lang is the highlight language (code only).
	Lang string `json:"lang,omitempty"`

	// Ordered distinguishes an ordered list from an unordered one (list only).
	Ordered bool `json:"ordered,omitempty"`
	// Items holds list items; each item is itself a list of blocks (list only).
	Items [][]Block `json:"items,omitempty"`

	// Content carries inline payload (paragraph, heading, quote).
	Content []Inline `json:"content,omitempty"`

	// Children carries nested blocks (callout, definition, and other
	// semantic containers).
	Children []Block `json:"children,omitempty"`

	// Columns and Rows describe a table (table only).
	Columns []TableColumn `json:"columns,omitempty"`
	Rows    []TableRow    `json:"rows,omitempty"`

	// Math carries an equation (equation only).
	Math *Equation `json:"math,omitempty"`

	// Src and Alt describe an image (image only).
	Src string `json:"src,omitempty"`
	Alt string `json:"alt,omitempty"`
	// Caption is rendered as a Typst #figure caption (image and table only;
	// ADR 2026072602 DR5d wraps a table in #figure so it is numbered and
	// listable in the List of Tables regardless of whether it has one).
	Caption []Inline `json:"caption,omitempty"`
}

// Inline is an inline content node.
type Inline struct {
	Type    string   `json:"type"`
	Text    string   `json:"text,omitempty"`
	Content []Inline `json:"content,omitempty"`

	// URL is the link target (link only).
	URL string `json:"url,omitempty"`

	// Math carries inline math (math only).
	Math *Equation `json:"math,omitempty"`

	// Target addresses a block (cross_reference only).
	Target *RefTarget `json:"target,omitempty"`

	// CitationKey and Locator identify a bibliography entry (citation only).
	CitationKey string `json:"citation_key,omitempty"`
	Locator     string `json:"locator,omitempty"`
}

// TableColumn declares one column of a table block.
type TableColumn struct {
	Key   string `json:"key"`
	Title string `json:"title"`
	// Align is "left", "center", or "right"; empty means "left".
	Align string `json:"align,omitempty"`
}

// TableRow is one row of a table block. Cells are keyed by TableColumn.Key;
// a key absent from Cells renders as an empty cell (spec §1.2 rule 5).
type TableRow struct {
	Cells map[string][]Inline `json:"cells"`
}

// Equation is the CDM v1.0 equation shape (spec §6.4): store the original
// source, store a normalized semantic math AST when parsing succeeds, and
// render from the AST when available, falling back to the original
// representation otherwise.
type Equation struct {
	Display     bool        `json:"display"`
	Original    *MathSource `json:"original,omitempty"`
	Normalized  *MathExpr   `json:"normalized,omitempty"`
	ParseStatus string      `json:"parse_status"` // success | failed | skipped
}

// MathSource is an equation's source as originally authored.
type MathSource struct {
	Format string `json:"format"` // latex | typst | asciimath
	Source string `json:"source"`
}

// MathExpr is one node of a normalized math AST: either an operator node
// (Op + Args) or a leaf (Type + Name/Value). Numeric literals are carried as
// exact decimal strings so they round-trip without binary floating-point
// loss (spec §6.4).
type MathExpr struct {
	Op   string     `json:"op,omitempty"`
	Args []MathExpr `json:"args,omitempty"`

	Type  string `json:"type,omitempty"`  // symbol | number
	Name  string `json:"name,omitempty"`  // symbol
	Value string `json:"value,omitempty"` // number, as an exact decimal string
}

// RefTarget addresses a block, optionally in another document. An empty
// DocumentKey means "this document" (spec §7).
type RefTarget struct {
	DocumentKey string `json:"document_key,omitempty"`
	BlockID     string `json:"block_id"`
}
