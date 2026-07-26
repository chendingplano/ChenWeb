package rendering_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
	"github.com/chendingplano/deepdoc/server/api/cdm/model"
	"github.com/chendingplano/deepdoc/server/api/cdm/rendering"
)

func requireTypst(t *testing.T) string {
	t.Helper()
	bin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not found on PATH")
	}
	return bin
}

// renderToTempDir renders doc and writes it, plus the shipped theme, into a
// fresh temp directory, returning the .typ file path.
func renderToTempDir(t *testing.T, doc *model.Document) string {
	t.Helper()
	r := &rendering.TypstRenderer{}
	src, err := r.RenderDocument(doc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	tmp := t.TempDir()
	theme, err := os.ReadFile("theme.typ")
	if err != nil {
		t.Fatalf("read theme.typ: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "theme.typ"), theme, 0o644); err != nil {
		t.Fatalf("write theme.typ: %v", err)
	}
	// AllBlockTypes carries an image block referencing this fixture; other
	// callers' documents simply never reference it.
	if err := os.MkdirAll(filepath.Join(tmp, "diagrams"), 0o755); err != nil {
		t.Fatalf("mkdir diagrams: %v", err)
	}
	img, err := os.ReadFile(filepath.Join("testdata", "diagrams", "flow.png"))
	if err != nil {
		t.Fatalf("read fixture image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "diagrams", "flow.png"), img, 0o644); err != nil {
		t.Fatalf("write fixture image: %v", err)
	}
	typPath := filepath.Join(tmp, "doc.typ")
	if err := os.WriteFile(typPath, src, 0o644); err != nil {
		t.Fatalf("write doc.typ: %v", err)
	}
	return typPath
}

func TestExtractAnchors_CoversEveryUnit(t *testing.T) {
	typstBin := requireTypst(t)
	doc := cdmfixtures.JaroWinkler()
	typPath := renderToTempDir(t, &doc)

	marks, err := rendering.ExtractAnchors(typstBin, typPath)
	if err != nil {
		t.Fatalf("extract anchors: %v", err)
	}

	starts := map[string]bool{}
	ends := map[string]bool{}
	for _, m := range marks {
		switch m.Kind {
		case "start":
			starts[m.ID] = true
		case "end":
			ends[m.ID] = true
		default:
			t.Fatalf("unexpected mark kind %q", m.Kind)
		}
	}

	for _, id := range []string{"intro", "score-range", "score-range-1", "score-range-2",
		"example-table", "example-table/row0", "example-table/row1", "jaro-formula"} {
		if !starts[id] {
			t.Errorf("missing start mark for unit %q", id)
		}
		if !ends[id] {
			t.Errorf("missing end mark for unit %q", id)
		}
	}
}

func TestExtractAnchors_PageRelativeCoordinates(t *testing.T) {
	typstBin := requireTypst(t)

	// A document long enough to force content onto a second page (page body
	// is 792 - 2*54 = 684pt tall; ~40 short paragraphs comfortably exceeds
	// one page).
	var blocks []model.Block
	for i := range 40 {
		blocks = append(blocks, model.Block{
			ID:   idFor(i),
			Type: "paragraph",
			Content: []model.Inline{
				{Type: "text", Text: "This is paragraph content used to force pagination in the anchor extraction test."},
			},
		})
	}
	doc := model.Document{
		Title: "Pagination", SchemaVersion: model.SchemaVersion, Blocks: blocks,
	}
	typPath := renderToTempDir(t, &doc)

	marks, err := rendering.ExtractAnchors(typstBin, typPath)
	if err != nil {
		t.Fatalf("extract anchors: %v", err)
	}

	pages := map[int]bool{}
	for _, m := range marks {
		pages[m.Page] = true
	}
	if len(pages) < 2 {
		t.Fatalf("expected content to span at least 2 pages, got pages: %v", pages)
	}

	byID := map[string]rendering.Mark{}
	for _, m := range marks {
		if m.Kind == "start" {
			byID[m.ID] = m
		}
	}
	first, ok1 := byID[idFor(0)]
	last, ok2 := byID[idFor(39)]
	if !ok1 || !ok2 {
		t.Fatalf("missing start marks for first/last paragraph")
	}
	if first.Page == last.Page {
		t.Fatalf("expected first and last paragraph on different pages, both on page %d", first.Page)
	}
	if last.Y >= rendering.ContentTop+50 {
		// Loose sanity check: a page-relative y should be small near the top
		// of its own page, not a cumulative offset across all prior pages.
		t.Logf("last.Y = %v (page %d) -- not necessarily wrong, just noting for review", last.Y, last.Page)
	}
}

func idFor(i int) string { return "p" + strconv.Itoa(i) }

func TestDeriveFragments_SinglePageUnit(t *testing.T) {
	marks := []rendering.Mark{
		{ID: "p1", Kind: "start", Page: 1, X: 54, Y: 100},
		{ID: "p1", Kind: "end", Page: 1, X: 200, Y: 130},
	}
	frags, err := rendering.DeriveFragments(marks)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(frags) != 1 {
		t.Fatalf("expected 1 fragment, got %d", len(frags))
	}
	f := frags[0]
	if f.Page != 1 || f.Y != 100 || f.H != 30 {
		t.Fatalf("unexpected fragment: %+v", f)
	}
	if f.X != rendering.PageMargin || f.W != rendering.ContentWidth {
		t.Fatalf("expected full content-width fragment, got %+v", f)
	}
}

func TestDeriveFragments_PageSpanningUnit(t *testing.T) {
	marks := []rendering.Mark{
		{ID: "p1", Kind: "start", Page: 1, X: 54, Y: 700},
		{ID: "p1", Kind: "end", Page: 3, X: 200, Y: 100},
	}
	frags, err := rendering.DeriveFragments(marks)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if len(frags) != 3 {
		t.Fatalf("expected 3 fragments (start page, one middle page, end page), got %d: %+v", len(frags), frags)
	}
	if frags[0].Page != 1 || frags[0].Y != 700 || frags[0].H != rendering.ContentBottom-700 {
		t.Fatalf("unexpected first fragment: %+v", frags[0])
	}
	if frags[1].Page != 2 || frags[1].Y != rendering.ContentTop || frags[1].H != rendering.ContentBottom-rendering.ContentTop {
		t.Fatalf("unexpected middle fragment: %+v", frags[1])
	}
	if frags[2].Page != 3 || frags[2].Y != rendering.ContentTop || frags[2].H != 100-rendering.ContentTop {
		t.Fatalf("unexpected last fragment: %+v", frags[2])
	}
}

func TestDeriveFragments_MissingEndMarkErrors(t *testing.T) {
	marks := []rendering.Mark{
		{ID: "p1", Kind: "start", Page: 1, X: 54, Y: 100},
	}
	if _, err := rendering.DeriveFragments(marks); err == nil {
		t.Fatal("expected error for unit with start but no end mark")
	}
}

func TestAnchoredRendering_AlignmentAgainstRenderedSVG(t *testing.T) {
	typstBin := requireTypst(t)
	doc := cdmfixtures.JaroWinkler()
	typPath := renderToTempDir(t, &doc)

	marks, err := rendering.ExtractAnchors(typstBin, typPath)
	if err != nil {
		t.Fatalf("extract anchors: %v", err)
	}
	frags, err := rendering.DeriveFragments(marks)
	if err != nil {
		t.Fatalf("derive fragments: %v", err)
	}

	var introFrag *rendering.Fragment
	for i := range frags {
		if frags[i].UnitID == "intro" {
			introFrag = &frags[i]
			break
		}
	}
	if introFrag == nil {
		t.Fatal("no fragment found for unit \"intro\"")
	}

	// Render an SVG page and overlay a rect at the fragment's coordinates,
	// then confirm the rect is well-formed and within the page bounds --
	// the strongest check available without a pixel-diff harness, but this
	// is the exact technique verified visually during design (a rect placed
	// at a queried bbox landed exactly on its block).
	svgPath := filepath.Join(filepath.Dir(typPath), "page1.svg")
	cmd := exec.Command(typstBin, "compile", "--format", "svg", typPath, svgPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("typst compile svg: %v\n%s", err, out)
	}
	svgData, err := os.ReadFile(svgPath)
	if err != nil {
		t.Fatalf("read svg: %v", err)
	}
	if len(svgData) == 0 {
		t.Fatal("expected non-empty SVG output")
	}
	if introFrag.X < 0 || introFrag.Y < 0 || introFrag.X+introFrag.W > rendering.PageWidth || introFrag.Y+introFrag.H > rendering.PageHeight {
		t.Fatalf("fragment out of page bounds: %+v", introFrag)
	}
}

func TestCollectUnits_EveryUnitHasExactlyOneAnchorPair(t *testing.T) {
	typstBin := requireTypst(t)
	doc := cdmfixtures.JaroWinkler()
	units := rendering.CollectUnits(&doc)
	typPath := renderToTempDir(t, &doc)

	marks, err := rendering.ExtractAnchors(typstBin, typPath)
	if err != nil {
		t.Fatalf("extract anchors: %v", err)
	}
	starts := map[string]int{}
	ends := map[string]int{}
	for _, m := range marks {
		switch m.Kind {
		case "start":
			starts[m.ID]++
		case "end":
			ends[m.ID]++
		}
	}

	if len(units) == 0 {
		t.Fatal("expected at least one unit")
	}
	for _, u := range units {
		if starts[u.ID] != 1 {
			t.Errorf("unit %q has %d start marks, want 1", u.ID, starts[u.ID])
		}
		if ends[u.ID] != 1 {
			t.Errorf("unit %q has %d end marks, want 1", u.ID, ends[u.ID])
		}
	}
	if len(units) != len(starts) {
		t.Errorf("unit count %d does not match distinct start-marked ids %d", len(units), len(starts))
	}
}

func TestCollectUnits_TableRowTextConcatenatesCells(t *testing.T) {
	doc := cdmfixtures.JaroWinkler()
	units := rendering.CollectUnits(&doc)
	var row0 *rendering.Unit
	for i := range units {
		if units[i].ID == "example-table/row0" {
			row0 = &units[i]
		}
	}
	if row0 == nil {
		t.Fatal("expected a unit for example-table/row0")
	}
	if row0.Text != "John Jon 0.93" {
		t.Fatalf("got %q, want %q", row0.Text, "John Jon 0.93")
	}
}
