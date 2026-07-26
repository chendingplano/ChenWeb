package rendering_test

import (
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
)

// queryTypst runs `typst query <path> <selector>` and decodes the JSON array
// of matched element dictionaries. Unlike ExtractAnchors, this queries
// Typst's own element tree (headings, figures, equations), not our metadata
// label, so it needs no --field value: most elements here carry no "value"
// field at all.
func queryTypst(t *testing.T, typstBin, path, selector string) []map[string]any {
	t.Helper()
	out, err := exec.Command(typstBin, "query", path, selector).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			t.Fatalf("typst query %q failed: %v\n%s", selector, err, ee.Stderr)
		}
		t.Fatalf("typst query %q failed: %v", selector, err)
	}
	var result []map[string]any
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("decode typst query output: %v\nraw: %s", err, out)
	}
	return result
}

// TestRenderDocument_HeadingsAreRealTypstHeadings guards against a
// regression where headings render as literal "=" text rather than real
// Typst heading elements: renderBlocks prefixes every block with a
// #mark(id, "start") call on the same line, and "=" markup sugar only parses
// as a heading when it is the first token on its line -- verified against the
// real compiler that a mark-prefixed "= Sub" produces zero query(heading)
// matches. A content heading must register as a query(heading) match, with
// the right level, numbered and outlined; the document title must not
// (design DR5d: only Typst-numbered content headings belong in the Contents
// outline).
func TestRenderDocument_HeadingsAreRealTypstHeadings(t *testing.T) {
	typstBin := requireTypst(t)
	doc := cdmfixtures.AllBlockTypes()
	typPath := renderToTempDir(t, &doc)

	headings := queryTypst(t, typstBin, typPath, "heading")

	var titleHeading, contentHeading map[string]any
	for _, h := range headings {
		body, _ := h["body"].(map[string]any)
		text, _ := body["text"].(string)
		switch text {
		case doc.Title:
			titleHeading = h
		case "Title Heading":
			contentHeading = h
		}
	}

	if titleHeading == nil {
		t.Fatalf("expected the document title to appear as a heading element, got: %+v", headings)
	}
	if titleHeading["outlined"] != false {
		t.Errorf("expected the document title to be excluded from outlines, got outlined=%v", titleHeading["outlined"])
	}
	if titleHeading["numbering"] != nil {
		t.Errorf("expected the document title to carry no numbering, got %v", titleHeading["numbering"])
	}

	if contentHeading == nil {
		t.Fatalf(`expected block h1 ("Title Heading") to register as a real Typst heading, got: %+v`, headings)
	}
	if contentHeading["level"] != float64(1) {
		t.Errorf("expected level 1, got %v", contentHeading["level"])
	}
	if contentHeading["outlined"] != true {
		t.Errorf("expected the content heading to be outlined, got %v", contentHeading["outlined"])
	}
	if contentHeading["numbering"] == nil {
		t.Errorf("expected the content heading to carry a numbering pattern, got nil")
	}
}

// TestRenderDocument_TableIsNumberedFigure guards DR5d's table-as-figure
// wrapping: a table block must be found by figure.where(kind: table), which
// is what makes it numbered and listable in the List of Tables. A bare
// #table(...) call (the pre-DR5d shape) is invisible to that selector.
func TestRenderDocument_TableIsNumberedFigure(t *testing.T) {
	typstBin := requireTypst(t)
	doc := cdmfixtures.JaroWinkler()
	typPath := renderToTempDir(t, &doc)

	tables := queryTypst(t, typstBin, typPath, "figure.where(kind: table)")
	if len(tables) != 1 {
		t.Fatalf("expected exactly 1 table figure, got %d: %+v", len(tables), tables)
	}
	if tables[0]["numbering"] == nil {
		t.Errorf("expected the table figure to carry a numbering pattern, got nil")
	}
	if tables[0]["outlined"] != true {
		t.Errorf("expected the table figure to be outline-eligible, got %v", tables[0]["outlined"])
	}
}

// TestRenderDocument_OutlinesNonEmptyForPopulatedFixtures is the guard ADR
// 2026072602's DR5d Implementation section calls for: an empty #outline(...)
// compiles cleanly, so a build-only check cannot catch a regression that
// silently empties the TOC or a list. Both shared fixtures carry at least one
// entry for the outlines asserted on here. AllBlockTypes' eq1 is deliberately
// excluded from the formula check: it is an inline equation (Display: false),
// so it must NOT appear in math.equation.where(block: true) -- that
// exclusion is itself part of the design, not an oversight.
func TestRenderDocument_OutlinesNonEmptyForPopulatedFixtures(t *testing.T) {
	typstBin := requireTypst(t)

	jw := cdmfixtures.JaroWinkler()
	jwPath := renderToTempDir(t, &jw)
	if got := queryTypst(t, typstBin, jwPath, "figure.where(kind: table)"); len(got) == 0 {
		t.Error("jaro-winkler: expected a non-empty List of Tables")
	}
	if got := queryTypst(t, typstBin, jwPath, "math.equation.where(block: true)"); len(got) == 0 {
		t.Error("jaro-winkler: expected a non-empty List of Formulas")
	}

	abt := cdmfixtures.AllBlockTypes()
	abtPath := renderToTempDir(t, &abt)
	if got := queryTypst(t, typstBin, abtPath, "heading.where(outlined: true)"); len(got) == 0 {
		t.Error("all-block-types: expected a non-empty Contents outline")
	}
	if got := queryTypst(t, typstBin, abtPath, "figure.where(kind: image)"); len(got) == 0 {
		t.Error("all-block-types: expected a non-empty List of Figures")
	}
	if got := queryTypst(t, typstBin, abtPath, "figure.where(kind: table)"); len(got) == 0 {
		t.Error("all-block-types: expected a non-empty List of Tables")
	}
}
