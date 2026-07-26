package rendering

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/cdm/model"
)

// Page geometry for anchored rendering (spec §5.7). These are fixed rather
// than left to Typst defaults so that fragment derivation (DeriveFragments)
// can compute x/w from known constants instead of re-measuring content --
// measure() was verified unreliable for content that spans a page break
// (design D11), and a full per-unit remeasurement scheme is out of scope for
// Phase 1. All units are points.
const (
	PageWidth     = 612.0 // US Letter
	PageHeight    = 792.0
	PageMargin    = 54.0 // 0.75in
	ContentWidth  = PageWidth - 2*PageMargin
	ContentTop    = PageMargin
	ContentBottom = PageHeight - PageMargin
)

// pageSetupTypst is emitted at the top of every rendered document so the
// geometry above matches what Typst actually lays out.
const pageSetupTypst = `#set page(width: 612pt, height: 792pt, margin: 54pt)`

// anchorPreludeTypst defines the mark() function and its backing state.
// mark(id, "start") / mark(id, "end") record here().position() into __anchors
// without attaching to any content, so it does not interact with content
// labels (see renderBlocks) or their #block[...] wrapping requirement for
// lists.
const anchorPreludeTypst = `#let __anchors = state("__anchors", ())
#let mark(id, kind) = context {
  let p = here().position()
  __anchors.update(a => a + ((id: id, kind: kind, page: p.page, x: p.x / 1pt, y: p.y / 1pt),))
}`

// anchorTrailerTypst dumps the accumulated marks as queryable metadata. The
// label name is arbitrary; ExtractAnchors must query the same one.
const (
	anchorLabelName = "cdm-anchors"
)

var anchorTrailerTypst = fmt.Sprintf(`#context [#metadata(__anchors.final())#label(%q)]`, anchorLabelName)

// Mark is one raw position sample recorded by the Typst mark() function.
type Mark struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // "start" | "end"
	Page int    `json:"page"`
	X    float64
	Y    float64
}

// ExtractAnchors invokes `typst query` against a compiled document's source
// file and decodes the accumulated marks (spec §5.7: anchor extraction via
// typst query, verified against Typst 0.14.2). typstPath is the path to the
// rendered .typ file (with its theme and any assets available alongside it,
// exactly as required to compile it).
func ExtractAnchors(typstBin, typstPath string) ([]Mark, error) {
	if typstBin == "" {
		typstBin = "typst"
	}
	cmd := exec.Command(typstBin, "query", typstPath, "<"+anchorLabelName+">", "--field", "value")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("cdm: typst query failed: %w: %s", err, stderr.String())
	}

	// `typst query --field value` on a single matched element returns
	// `[<value>]`; the value here is the array __anchors.final() produced,
	// so the decoded shape is [][]Mark with exactly one outer element.
	var wrapped [][]Mark
	if err := json.Unmarshal(stdout.Bytes(), &wrapped); err != nil {
		return nil, fmt.Errorf("cdm: decode typst query output: %w", err)
	}
	if len(wrapped) != 1 {
		return nil, fmt.Errorf("cdm: expected exactly one <%s> metadata element, got %d", anchorLabelName, len(wrapped))
	}
	return wrapped[0], nil
}

// Fragment is one highlight rectangle on one page, in points, in the same
// coordinate space as the rendered SVG page (spec §5.7).
type Fragment struct {
	UnitID string
	Page   int
	X, Y   float64
	W, H   float64
}

// DeriveFragments pairs each unit's start/end marks into one fragment per
// page it occupies (spec "Units spanning a page break"): a single-page unit
// yields one fragment; a unit whose start and end land on different pages
// yields one fragment per page, using the known content-box edges (ContentTop
// / ContentBottom) as the fragment boundary on the pages it doesn't start or
// end on.
//
// Known Phase 1 precision limitation, confirmed visually (a rect painted at
// a derived fragment's coordinates over a real rendered table): every
// fragment's X and W are the page's full content margin/width, not the
// rendered element's actual width. This is exactly correct for full-width
// flow content (paragraphs, headings) -- verified pixel-accurate -- but
// overshoots visually for narrower elements such as tables that don't fill
// the content width, and undershoots the true left edge for indented list
// items. The Y/H bounds (the harder problem: pagination) are unaffected and
// verified pixel-accurate in both cases. Fixing X/W precision requires a
// per-unit measured width, which is deferred (see design D11's rejection of
// measure() for the page-spanning case) rather than solved here.
func DeriveFragments(marks []Mark) ([]Fragment, error) {
	starts := map[string]Mark{}
	ends := map[string]Mark{}
	var order []string
	for _, m := range marks {
		switch m.Kind {
		case "start":
			if _, seen := starts[m.ID]; !seen {
				order = append(order, m.ID)
			}
			starts[m.ID] = m
		case "end":
			ends[m.ID] = m
		default:
			return nil, fmt.Errorf("cdm: unknown mark kind %q for unit %q", m.Kind, m.ID)
		}
	}

	var frags []Fragment
	for _, id := range order {
		start, ok := starts[id]
		if !ok {
			continue
		}
		end, ok := ends[id]
		if !ok {
			return nil, fmt.Errorf("cdm: unit %q has a start mark but no end mark", id)
		}

		if start.Page == end.Page {
			frags = append(frags, Fragment{
				UnitID: id, Page: start.Page,
				X: PageMargin, Y: start.Y,
				W: ContentWidth, H: end.Y - start.Y,
			})
			continue
		}

		// Page-spanning: one fragment from the start down to the content
		// box's bottom edge, one full-content-height fragment for each
		// wholly-contained intermediate page, and one fragment from the
		// content box's top edge down to the end mark.
		frags = append(frags, Fragment{
			UnitID: id, Page: start.Page,
			X: PageMargin, Y: start.Y,
			W: ContentWidth, H: ContentBottom - start.Y,
		})
		for p := start.Page + 1; p < end.Page; p++ {
			frags = append(frags, Fragment{
				UnitID: id, Page: p,
				X: PageMargin, Y: ContentTop,
				W: ContentWidth, H: ContentBottom - ContentTop,
			})
		}
		frags = append(frags, Fragment{
			UnitID: id, Page: end.Page,
			X: PageMargin, Y: ContentTop,
			W: ContentWidth, H: end.Y - ContentTop,
		})
	}
	return frags, nil
}

// Unit is one anchorable unit of a document: something that gets its own
// mark("id", "start")/mark("id", "end") pair in the rendered Typst source,
// and becomes exactly one line-file line (spec §10.1, §5.7). Every block
// visited by renderBlocks is a unit, and each row of a table is additionally
// its own unit (ID synthesized by tableRowUnitID, since TableRow carries no
// id of its own).
type Unit struct {
	ID   string
	Type string // the block's Type, or "table-row" for a synthesized row unit
	Text string // plain text content, for the line file
}

// CollectUnits walks doc in the same order renderBlocks emits marks, so the
// returned units line up 1:1 with what ExtractAnchors will find. Used by the
// anchor-coverage check and by line-file generation.
func CollectUnits(doc *model.Document) []Unit {
	var units []Unit
	var walk func(blocks []model.Block)
	walk = func(blocks []model.Block) {
		for _, b := range blocks {
			text := model.PlainText(b.Content)
			if b.Type == "code" {
				text = b.Text
			}
			units = append(units, Unit{ID: b.ID, Type: b.Type, Text: text})

			if b.Type == "table" {
				for i, row := range b.Rows {
					var cells []string
					for _, col := range b.Columns {
						cells = append(cells, model.PlainText(row.Cells[col.Key]))
					}
					units = append(units, Unit{
						ID: tableRowUnitID(b.ID, i), Type: "table-row",
						Text: strings.Join(cells, " "),
					})
				}
			}

			walk(b.Children)
			for _, item := range b.Items {
				walk(item)
			}
		}
	}
	walk(doc.Blocks)
	return units
}

// tableRowUnitID synthesizes the anchor/line-file unit id for one row of a
// table block. TableRow has no id of its own (spec §5.2); rows are anchored
// as sub-units of their table using this derived name.
func tableRowUnitID(tableBlockID string, rowIndex int) string {
	return fmt.Sprintf("%s/row%d", tableBlockID, rowIndex)
}
