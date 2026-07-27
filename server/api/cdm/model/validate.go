package model

import (
	"fmt"
	"strings"
)

// blockTypes is the Phase 1 block type vocabulary (spec §2, §3). "warning" is
// deliberately absent: severity is a callout role, not a block type.
var blockTypes = map[string]bool{
	"paragraph": true,
	"heading":   true,
	"list":      true,
	"table":     true,
	"equation":  true,
	"code":      true,
	"quote":     true,
	"image":     true,
	"callout":   true,
}

// inlineTypes is the Phase 1 inline type vocabulary (spec §2).
var inlineTypes = map[string]bool{
	"text":            true,
	"line_break":      true,
	"strong":          true,
	"emphasis":        true,
	"code":            true,
	"link":            true,
	"math":            true,
	"citation":        true,
	"cross_reference": true,
}

// calloutRoles is the callout severity vocabulary (spec §3).
var calloutRoles = map[string]bool{
	"note":      true,
	"tip":       true,
	"important": true,
	"warning":   true,
	"caution":   true,
}

var equationFormats = map[string]bool{
	"latex":     true,
	"typst":     true,
	"asciimath": true,
}

var parseStatuses = map[string]bool{
	"success": true,
	"failed":  true,
	"skipped": true,
}

// ValidationError reports every invariant violation found in a document
// (spec §1.2). The validator accumulates all violations rather than
// stopping at the first.
type ValidationError struct {
	Violations []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("cdm: document invalid (%d violation(s)): %s",
		len(e.Violations), strings.Join(e.Violations, "; "))
}

// Validate checks doc against every CDM v1.0 content-model invariant
// (spec §1.2) and returns a *ValidationError describing all violations
// found, or nil if the document is valid.
func Validate(doc *Document) error {
	v := &validator{seenIDs: map[string]bool{}}

	if doc.SchemaVersion == "" {
		v.fail("schema_version is required")
	} else if doc.SchemaVersion != SchemaVersion {
		v.fail("unrecognized schema_version %q (expected %q)", doc.SchemaVersion, SchemaVersion)
	}

	v.validateBlocks(doc.Blocks, "")

	if len(v.violations) == 0 {
		return nil
	}
	return &ValidationError{Violations: v.violations}
}

type validator struct {
	violations []string
	seenIDs    map[string]bool
}

func (v *validator) fail(format string, args ...any) {
	v.violations = append(v.violations, fmt.Sprintf(format, args...))
}

func (v *validator) validateBlocks(blocks []Block, parentDesc string) {
	for i, b := range blocks {
		v.validateBlock(b, i, parentDesc)
	}
}

func (v *validator) validateBlock(b Block, index int, parentDesc string) {
	desc := b.ID
	if desc == "" {
		desc = fmt.Sprintf("<no id, position %d in %s>", index, describeParent(parentDesc))
		v.fail("block at position %d in %s has an empty id", index, describeParent(parentDesc))
	} else {
		if v.seenIDs[desc] {
			v.fail("duplicate block id %q", desc)
		}
		v.seenIDs[desc] = true
	}

	if b.Type == "" {
		v.fail("block %q has no type", desc)
	} else if b.Type == "warning" {
		v.fail("block %q has type \"warning\", which is not a block type; use type \"callout\" with role \"warning\" instead", desc)
	} else if !blockTypes[b.Type] {
		v.fail("block %q has unsupported type %q", desc, b.Type)
	}

	if b.Role != "" && !calloutRoles[b.Role] {
		v.fail("block %q has unsupported role %q", desc, b.Role)
	}

	v.validateExclusivity(b, desc)

	if b.Type == "heading" && (b.Level < 1 || b.Level > 6) {
		v.fail("heading %q has level %d, outside the range 1-6", desc, b.Level)
	}

	if b.Type == "table" {
		v.validateTable(b, desc)
	}

	if b.Type == "equation" {
		v.validateEquation(b, desc)
	}

	for _, inl := range b.Content {
		v.validateInline(inl, desc)
	}
	for _, inl := range b.Caption {
		v.validateInline(inl, desc)
	}

	v.validateBlocks(b.Children, desc)
	for _, item := range b.Items {
		v.validateBlocks(item, desc)
	}
}

func describeParent(parentDesc string) string {
	if parentDesc == "" {
		return "document"
	}
	return fmt.Sprintf("block %q", parentDesc)
}

// validateExclusivity enforces spec §1.2 rule 4: content XOR children XOR
// items, never more than one of the three populated.
func (v *validator) validateExclusivity(b Block, desc string) {
	var populated []string
	if len(b.Content) > 0 {
		populated = append(populated, "content")
	}
	if len(b.Children) > 0 {
		populated = append(populated, "children")
	}
	if len(b.Items) > 0 {
		populated = append(populated, "items")
	}
	if len(populated) > 1 {
		v.fail("block %q populates more than one of content/children/items: %s",
			desc, strings.Join(populated, ", "))
	}
}

// validateTable enforces spec §1.2 rule 5: unique non-empty column keys, and
// row cell keys a subset of the declared columns.
func (v *validator) validateTable(b Block, desc string) {
	colKeys := map[string]bool{}
	for _, col := range b.Columns {
		if col.Key == "" {
			v.fail("table %q has a column with an empty key", desc)
			continue
		}
		if colKeys[col.Key] {
			v.fail("table %q declares duplicate column key %q", desc, col.Key)
		}
		colKeys[col.Key] = true
	}

	for i, row := range b.Rows {
		for key := range row.Cells {
			if !colKeys[key] {
				v.fail("table %q row %d has cell key %q not declared in columns", desc, i, key)
			}
		}
	}
}

// validateEquation enforces spec §1.2 rule 6.
func (v *validator) validateEquation(b Block, desc string) {
	if b.Math == nil {
		v.fail("equation %q has no math", desc)
		return
	}

	if b.Math.ParseStatus == "" {
		v.fail("equation %q has no parse_status", desc)
	} else if !parseStatuses[b.Math.ParseStatus] {
		v.fail("equation %q has unsupported parse_status %q (must be one of success, failed, skipped)",
			desc, b.Math.ParseStatus)
	}

	if b.Math.Normalized == nil && b.Math.Original == nil {
		v.fail("equation %q has neither normalized nor original", desc)
	}

	if b.Math.Original != nil && !equationFormats[b.Math.Original.Format] {
		v.fail("equation %q has original with unsupported format %q", desc, b.Math.Original.Format)
	}
}

func (v *validator) validateInline(inl Inline, blockDesc string) {
	if !inlineTypes[inl.Type] {
		v.fail("block %q has inline node with unsupported type %q", blockDesc, inl.Type)
		return
	}
	for _, child := range inl.Content {
		v.validateInline(child, blockDesc)
	}
}
