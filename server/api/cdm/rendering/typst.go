// Package rendering implements the CDM v1.0 Typst renderer (spec §5.2,
// §5.6): deterministic translation of a canonical document into Typst
// source. It depends only on cdm/model, so it can be promoted to shared/go
// as a package move (design D4).
package rendering

import (
	"fmt"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/cdm/model"
)

// TypstRenderer renders a canonical Document to Typst source.
type TypstRenderer struct {
	// Theme is the Typst theme file imported at the top of the output
	// (spec §5.3/§5.4). Empty means "theme.typ", the default shipped with
	// this package.
	Theme string
}

// RenderDocument emits Typst source for doc: explicit page geometry, the
// anchor-mark prelude, a theme import, the document title as a level-1
// heading, the rendered block body, and the anchor trailer (spec §5.2,
// §5.7). Anchored rendering is not optional output — every document carries
// the mark() calls that make navigate-and-highlight possible.
func (r *TypstRenderer) RenderDocument(doc *model.Document) ([]byte, error) {
	theme := r.Theme
	if theme == "" {
		theme = "theme.typ"
	}

	var out strings.Builder
	out.WriteString(pageSetupTypst + "\n")
	out.WriteString(anchorPreludeTypst + "\n\n")
	fmt.Fprintf(&out, "#import %q: *\n\n", theme)
	fmt.Fprintf(&out, "= %s\n\n", escapeTypst(doc.Title))

	body, err := renderBlocks(doc.Blocks)
	if err != nil {
		return nil, err
	}
	out.WriteString(body)
	out.WriteString("\n\n")
	out.WriteString(anchorTrailerTypst)
	out.WriteString("\n")

	return []byte(out.String()), nil
}

// renderBlocks renders a sequence of sibling blocks, separated by a blank
// line, in a single deterministic pass. Every block's output is suffixed
// with a Typst label matching its own id (`<id>`), verified empirically to
// attach cleanly to paragraphs, list items, tables, equations, and
// function-call blocks alike. This is what makes a cross_reference
// resolvable (see renderInline). Every block is additionally wrapped in a
// mark("id", "start")/mark("id", "end") pair (anchors.go) for position
// extraction; marks attach to nothing, so they never interact with the
// content-label collision that lists require #block[...] to avoid.
func renderBlocks(blocks []model.Block) (string, error) {
	var out strings.Builder
	for _, b := range blocks {
		rendered, err := renderBlock(b)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&out, "#mark(%q, \"start\")%s <%s>#mark(%q, \"end\")\n\n", b.ID, rendered, b.ID, b.ID)
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// renderBlock dispatches on b.Type. Unsupported types return an error naming
// the offending block rather than being skipped or silently emitting partial
// output (spec "Unsupported constructs fail loudly").
func renderBlock(b model.Block) (string, error) {
	switch b.Type {
	case "heading":
		content, err := renderInlines(b.Content)
		if err != nil {
			return "", fmt.Errorf("block %q: %w", b.ID, err)
		}
		return strings.Repeat("=", b.Level) + " " + content, nil

	case "paragraph":
		content, err := renderInlines(b.Content)
		if err != nil {
			return "", fmt.Errorf("block %q: %w", b.ID, err)
		}
		return content, nil

	case "quote":
		content, err := renderInlines(b.Content)
		if err != nil {
			return "", fmt.Errorf("block %q: %w", b.ID, err)
		}
		return "#quote(block: true)[" + content + "]", nil

	case "code":
		return fmt.Sprintf("#raw(block: true, lang: %q, %q)", b.Lang, b.Text), nil

	case "equation":
		return renderEquationBlock(b)

	case "list":
		return renderList(b)

	case "table":
		return renderTable(b)

	case "image":
		caption, err := renderInlines(b.Caption)
		if err != nil {
			return "", fmt.Errorf("block %q: %w", b.ID, err)
		}
		return fmt.Sprintf("#figure(image(%q), caption: [%s])", b.Src, caption), nil

	case "callout":
		return renderCallout(b)

	default:
		return "", fmt.Errorf("block %q: unsupported block type %q", b.ID, b.Type)
	}
}

func renderEquationBlock(b model.Block) (string, error) {
	if b.Math == nil {
		return "", fmt.Errorf("block %q: equation has no math", b.ID)
	}
	expr, err := renderMath(b.Math)
	if err != nil {
		return "", fmt.Errorf("block %q: %w", b.ID, err)
	}
	if b.Math.Display {
		return "$ " + expr + " $", nil
	}
	return "$" + expr + "$", nil
}

// renderList wraps its items in an explicit #block[...] container. This is
// required, not cosmetic: renderBlocks attaches a Typst label matching the
// list's own id immediately after whatever this function returns, and
// verified against the real compiler, a trailing label after bare list
// markup attaches to the *last item* (Typst: "content labelled multiple
// times... only the last label is used"), silently discarding that item's
// own label. Wrapping in #block[...] gives the list's label a container to
// attach to instead of colliding with a sibling.
func renderList(b model.Block) (string, error) {
	marker := "-"
	if b.Ordered {
		marker = "+"
	}

	var items strings.Builder
	for _, item := range b.Items {
		body, err := renderBlocks(item)
		if err != nil {
			return "", err
		}
		items.WriteString(marker + " " + indentContinuation(body) + "\n")
	}
	return "#block[\n" + strings.TrimRight(items.String(), "\n") + "\n]", nil
}

// indentContinuation indents every line after the first by two spaces, so a
// multi-block list item's later lines stay inside the item in Typst's list
// syntax.
func indentContinuation(s string) string {
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = "  " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func renderTable(b model.Block) (string, error) {
	aligns := make([]string, 0, len(b.Columns))
	for _, col := range b.Columns {
		align := col.Align
		if align == "" {
			align = "left"
		}
		aligns = append(aligns, align)
	}

	var out strings.Builder
	out.WriteString("#table(\n")
	fmt.Fprintf(&out, "  columns: %d,\n", len(b.Columns))
	fmt.Fprintf(&out, "  align: (%s),\n", strings.Join(aligns, ", "))
	out.WriteString("  table.header(\n")
	for _, col := range b.Columns {
		fmt.Fprintf(&out, "    [*%s*],\n", escapeTypst(col.Title))
	}
	out.WriteString("  ),\n")

	// Cells are emitted by iterating the declared columns, never the row's
	// Cells map, so output order does not depend on Go map iteration order
	// (design D7: determinism is a tested property).
	//
	// A table() call's positional arguments are all cells (row-major, by
	// `columns:` count); inserting a bare #mark(...) call as an extra
	// argument would corrupt the table shape. Verified instead that a mark
	// embedded inside a cell's own content -- [#mark(...)text] -- records
	// its position without disturbing the layout, so each row's start/end
	// marks are embedded in its first and last cell respectively (spec §5.7:
	// table rows are anchorable units; a row has no id of its own, so
	// tableRowUnitID synthesizes one from the table's block id).
	for rowIdx, row := range b.Rows {
		unitID := tableRowUnitID(b.ID, rowIdx)
		for colIdx, col := range b.Columns {
			cellContent, err := renderInlines(row.Cells[col.Key])
			if err != nil {
				return "", fmt.Errorf("block %q: %w", b.ID, err)
			}
			prefix, suffix := "", ""
			if colIdx == 0 {
				prefix = fmt.Sprintf("#mark(%q, \"start\")", unitID)
			}
			if colIdx == len(b.Columns)-1 {
				suffix = fmt.Sprintf("#mark(%q, \"end\")", unitID)
			}
			fmt.Fprintf(&out, "  [%s%s%s],\n", prefix, cellContent, suffix)
		}
	}
	out.WriteString(")")
	return out.String(), nil
}

func renderCallout(b model.Block) (string, error) {
	body, err := renderBlocks(b.Children)
	if err != nil {
		return "", err
	}
	role := b.Role
	if role == "" {
		role = "note"
	}
	return fmt.Sprintf("#callout(%q, [%s], [%s])", role, escapeTypst(b.Title), body), nil
}

// renderInline renders one inline node.
//
// cross_reference renders as #link(label(...)), not #ref(label(...)):
// verified against the real Typst compiler that #ref only resolves
// Typst's own numbered/referenceable elements (headings, figures, equations
// with numbering enabled) and errors ("cannot reference text") on arbitrary
// content such as a plain paragraph or a table row. Since a CDM
// cross_reference can target any block, #link is the only mechanism that
// resolves universally; every block is labeled with its own id in
// renderBlocks for exactly this purpose.
//
// citation defers to Typst's own #cite machinery (spec §7).
func renderInline(n model.Inline) (string, error) {
	switch n.Type {
	case "text":
		return escapeTypst(n.Text), nil

	case "strong":
		inner, err := renderInlines(n.Content)
		if err != nil {
			return "", err
		}
		return "*" + inner + "*", nil

	case "emphasis":
		inner, err := renderInlines(n.Content)
		if err != nil {
			return "", err
		}
		return "_" + inner + "_", nil

	case "code":
		return fmt.Sprintf("#raw(%q)", n.Text), nil

	case "link":
		inner, err := renderInlines(n.Content)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("#link(%q)[%s]", n.URL, inner), nil

	case "math":
		if n.Math == nil {
			return "", fmt.Errorf("inline math node has no math")
		}
		expr, err := renderMath(n.Math)
		if err != nil {
			return "", err
		}
		return "$" + expr + "$", nil

	case "cross_reference":
		if n.Target == nil {
			return "", fmt.Errorf("cross_reference has no target")
		}
		if n.Target.DocumentKey != "" {
			return "", fmt.Errorf("cross-document cross_reference (target document_key %q) is not supported by this renderer; Phase 1 resolves same-document references only", n.Target.DocumentKey)
		}
		linkText := n.Target.BlockID
		if len(n.Content) > 0 {
			inner, err := renderInlines(n.Content)
			if err != nil {
				return "", err
			}
			linkText = inner
		} else {
			linkText = escapeTypst(linkText)
		}
		return fmt.Sprintf("#link(label(%q))[%s]", n.Target.BlockID, linkText), nil

	case "citation":
		// Not #cite(label(...)): verified against the real compiler that
		// #cite requires an actual #bibliography to be present in the
		// document and hard-errors ("the document does not contain a
		// bibliography") otherwise. Phase 1 builds no bibliography
		// infrastructure, and `reference` blocks (spec §7, the bibliography
		// entries a citation_key resolves against) are not even a supported
		// block type in this renderer (they are Phase 3 scope) — so a label
		// to link to would not exist either. Render plain, visible citation
		// text instead of a call that is guaranteed to fail.
		text := n.CitationKey
		if n.Locator != "" {
			text += ", " + n.Locator
		}
		return escapeTypst("[" + text + "]"), nil

	default:
		return "", fmt.Errorf("unsupported inline type %q", n.Type)
	}
}

func renderInlines(nodes []model.Inline) (string, error) {
	var out strings.Builder
	for _, n := range nodes {
		s, err := renderInline(n)
		if err != nil {
			return "", err
		}
		out.WriteString(s)
	}
	return out.String(), nil
}
