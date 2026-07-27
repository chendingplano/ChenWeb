package rendering_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/cdm/cdmfixtures"
	"github.com/chendingplano/deepdoc/server/api/cdm/model"
	"github.com/chendingplano/deepdoc/server/api/cdm/rendering"
)

func TestRenderDocument_Preamble(t *testing.T) {
	doc := cdmfixtures.JaroWinkler()
	r := &rendering.TypstRenderer{}
	out, err := r.RenderDocument(&doc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, `#set page(width: 612pt, height: 792pt, margin: 54pt)`) {
		t.Fatalf("expected explicit page geometry in preamble:\n%s", got)
	}
	if !strings.Contains(got, `#import "theme.typ": *`+"\n\n"+`#set heading(numbering: "1.1")`) {
		t.Fatalf("expected theme import followed by the numbering set rules:\n%s", got)
	}
	if !strings.Contains(got, "#heading(numbering: none, outlined: false)[Jaro-Winkler Similarity]") {
		t.Fatalf("expected the title heading, excluded from numbering and outline:\n%s", got)
	}
	if !strings.Contains(got, "#outline(title: [Contents])") ||
		!strings.Contains(got, "#outline(title: [List of Figures], target: figure.where(kind: image))") ||
		!strings.Contains(got, "#outline(title: [List of Tables], target: figure.where(kind: table))") ||
		!strings.Contains(got, "#outline(title: [List of Formulas], target: math.equation.where(block: true))") {
		t.Fatalf("expected TOC and figure/table/formula outlines after the title (design DR5d):\n%s", got)
	}
}

func TestRenderDocument_HeadingLevel(t *testing.T) {
	doc := model.Document{
		Title:         "T",
		SchemaVersion: model.SchemaVersion,
		Blocks: []model.Block{
			{ID: "h", Type: "heading", Level: 3, Content: []model.Inline{{Type: "text", Text: "Sub"}}},
		},
	}
	r := &rendering.TypstRenderer{}
	out, err := r.RenderDocument(&doc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(string(out), "#heading(level: 3)[Sub]") {
		t.Fatalf("expected a level-3 #heading(...) call, got:\n%s", out)
	}
}

func TestRenderDocument_ParagraphLineBreak(t *testing.T) {
	doc := model.Document{
		Title:         "T",
		SchemaVersion: model.SchemaVersion,
		Blocks: []model.Block{{
			ID:   "p1",
			Type: "paragraph",
			Content: []model.Inline{
				{Type: "text", Text: "first line"},
				{Type: "line_break"},
				{Type: "text", Text: "second line"},
			},
		}},
	}
	r := &rendering.TypstRenderer{}
	out, err := r.RenderDocument(&doc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(string(out), `first line#linebreak()second line`) {
		t.Fatalf("expected inline Typst line break in paragraph output, got:\n%s", out)
	}
}

func TestRenderDocument_ListMarkers(t *testing.T) {
	unordered := model.Document{
		Title: "T", SchemaVersion: model.SchemaVersion,
		Blocks: []model.Block{{
			ID: "l", Type: "list", Ordered: false,
			Items: [][]model.Block{
				{{ID: "i1", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "one"}}}},
			},
		}},
	}
	ordered := unordered
	ordered.Blocks = []model.Block{{
		ID: "l", Type: "list", Ordered: true,
		Items: [][]model.Block{
			{{ID: "i1", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "one"}}}},
		},
	}}

	r := &rendering.TypstRenderer{}
	uOut, err := r.RenderDocument(&unordered)
	if err != nil {
		t.Fatalf("render unordered: %v", err)
	}
	oOut, err := r.RenderDocument(&ordered)
	if err != nil {
		t.Fatalf("render ordered: %v", err)
	}
	if !strings.Contains(string(uOut), "- #mark(\"i1\", \"start\")one") {
		t.Fatalf("expected a '-' marker before the item text in unordered output:\n%s", uOut)
	}
	if !strings.Contains(string(oOut), "+ #mark(\"i1\", \"start\")one") {
		t.Fatalf("expected a '+' marker before the item text in ordered output:\n%s", oOut)
	}
}

func TestRenderDocument_CodeIsRawNotFence(t *testing.T) {
	doc := model.Document{
		Title: "T", SchemaVersion: model.SchemaVersion,
		Blocks: []model.Block{{ID: "c", Type: "code", Lang: "go", Text: "x := 1"}},
	}
	r := &rendering.TypstRenderer{}
	out, err := r.RenderDocument(&doc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got := string(out)
	if strings.Contains(got, "```") {
		t.Fatalf("expected no Markdown fence in Typst output:\n%s", got)
	}
	if !strings.Contains(got, `#raw(block: true, lang: "go", "x := 1")`) {
		t.Fatalf("expected #raw call, got:\n%s", got)
	}
}

func TestRenderDocument_CalloutNoPresentationProperties(t *testing.T) {
	doc := model.Document{
		Title: "T", SchemaVersion: model.SchemaVersion,
		Blocks: []model.Block{{
			ID: "c", Type: "callout", Role: "warning", Title: "Limitation",
			Children: []model.Block{
				{ID: "c-body", Type: "paragraph", Content: []model.Inline{{Type: "text", Text: "x"}}},
			},
		}},
	}
	r := &rendering.TypstRenderer{}
	out, err := r.RenderDocument(&doc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got := string(out)
	// Scoped to the callout's own output line: the document preamble
	// legitimately uses pt units for page geometry (spec §5.7 anchored
	// rendering), which is unrelated to this block's own presentation.
	calloutLine := ""
	for line := range strings.SplitSeq(got, "\n") {
		if strings.Contains(line, "#callout(") {
			calloutLine = line
			break
		}
	}
	if calloutLine == "" {
		t.Fatalf("expected a #callout( line in output:\n%s", got)
	}
	for _, forbidden := range []string{"font", "color", "fill:", "stroke:", "px", "pt"} {
		if strings.Contains(strings.ToLower(calloutLine), forbidden) {
			t.Fatalf("callout output should carry no presentation properties, found %q in:\n%s", forbidden, calloutLine)
		}
	}
	want := `#callout("warning", [Limitation], [#mark("c-body", "start")x <c-body>#mark("c-body", "end")])`
	if !strings.Contains(calloutLine, want) {
		t.Fatalf("expected semantic #callout call, got:\n%s", calloutLine)
	}
}

func TestRenderDocument_UnsupportedBlockTypeErrors(t *testing.T) {
	doc := model.Document{
		Title: "T", SchemaVersion: model.SchemaVersion,
		Blocks: []model.Block{{ID: "bad", Type: "horizontal_stack"}},
	}
	r := &rendering.TypstRenderer{}
	_, err := r.RenderDocument(&doc)
	if err == nil {
		t.Fatal("expected error for unsupported block type")
	}
	if !strings.Contains(err.Error(), `"bad"`) || !strings.Contains(err.Error(), "horizontal_stack") {
		t.Fatalf("expected error naming block id and type, got: %v", err)
	}
}

func TestRenderDocument_NestedFailureAttributedToInnerBlock(t *testing.T) {
	doc := model.Document{
		Title: "T", SchemaVersion: model.SchemaVersion,
		Blocks: []model.Block{{
			ID: "outer", Type: "callout",
			Children: []model.Block{
				{ID: "inner-bad", Type: "not-a-type"},
			},
		}},
	}
	r := &rendering.TypstRenderer{}
	_, err := r.RenderDocument(&doc)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), `"inner-bad"`) {
		t.Fatalf("expected error to name the inner block, got: %v", err)
	}
	if strings.Contains(err.Error(), `"outer"`) {
		t.Fatalf("error should not blame the enclosing callout, got: %v", err)
	}
}

func TestRenderDocument_TableCellOrderFollowsColumns(t *testing.T) {
	doc := model.Document{
		Title: "T", SchemaVersion: model.SchemaVersion,
		Blocks: []model.Block{{
			ID:   "t",
			Type: "table",
			Columns: []model.TableColumn{
				{Key: "a", Title: "A"},
				{Key: "b", Title: "B"},
				{Key: "c", Title: "C"},
			},
			Rows: []model.TableRow{
				{Cells: map[string][]model.Inline{
					"c": {{Type: "text", Text: "3"}},
					"a": {{Type: "text", Text: "1"}},
					"b": {{Type: "text", Text: "2"}},
				}},
			},
		}},
	}
	r := &rendering.TypstRenderer{}
	out, err := r.RenderDocument(&doc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	got := string(out)
	// Cell 1 (column a) carries the row's leading mark() call, so match on
	// the digit itself rather than a bare "[1]", which no longer appears.
	iA := strings.Index(got, "start\")1")
	iB := strings.Index(got, "[2]")
	iC := strings.Index(got, "3#mark")
	if iA < 0 || iB < 0 || iC < 0 || !(iA < iB && iB < iC) {
		t.Fatalf("expected cells in column order a,b,c regardless of map order, got:\n%s", got)
	}
}

func TestRenderDocument_Deterministic100Runs(t *testing.T) {
	doc := cdmfixtures.AllBlockTypes()
	r := &rendering.TypstRenderer{}
	first, err := r.RenderDocument(&doc)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	for i := range 100 {
		out, err := r.RenderDocument(&doc)
		if err != nil {
			t.Fatalf("render iteration %d: %v", i, err)
		}
		if string(out) != string(first) {
			t.Fatalf("output diverged at iteration %d", i)
		}
	}
}

func TestRenderDocument_GoldenFiles(t *testing.T) {
	cases := map[string]model.Document{
		"jaro-winkler":    cdmfixtures.JaroWinkler(),
		"all-block-types": cdmfixtures.AllBlockTypes(),
	}
	r := &rendering.TypstRenderer{}
	for name, doc := range cases {
		t.Run(name, func(t *testing.T) {
			out, err := r.RenderDocument(&doc)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			goldenPath := filepath.Join("testdata", name+".typ.golden")
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden file %s: %v", goldenPath, err)
			}
			if string(out) != string(want) {
				t.Fatalf("rendered output does not match golden file %s\n--- got ---\n%s\n--- want ---\n%s",
					goldenPath, out, want)
			}
		})
	}
}

// TestGoldenFilesCompileWithTypst proves the emitted Typst source is valid,
// not merely stable: it invokes the real `typst` binary against each golden
// file plus the shipped theme.typ. Skips if typst is not on PATH.
func TestGoldenFilesCompileWithTypst(t *testing.T) {
	typstBin, err := exec.LookPath("typst")
	if err != nil {
		t.Skip("typst not found on PATH")
	}

	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}

	tmp := t.TempDir()
	themeSrc, err := os.ReadFile("theme.typ")
	if err != nil {
		t.Fatalf("read theme.typ: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "diagrams"), 0o755); err != nil {
		t.Fatalf("mkdir diagrams: %v", err)
	}
	imgSrc, err := os.ReadFile(filepath.Join("testdata", "diagrams", "flow.png"))
	if err != nil {
		t.Fatalf("read fixture image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "diagrams", "flow.png"), imgSrc, 0o644); err != nil {
		t.Fatalf("write fixture image: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "theme.typ"), themeSrc, 0o644); err != nil {
		t.Fatalf("write theme.typ: %v", err)
	}

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".typ.golden") {
			continue
		}
		t.Run(e.Name(), func(t *testing.T) {
			src, err := os.ReadFile(filepath.Join("testdata", e.Name()))
			if err != nil {
				t.Fatalf("read %s: %v", e.Name(), err)
			}
			inPath := filepath.Join(tmp, strings.TrimSuffix(e.Name(), ".golden"))
			if err := os.WriteFile(inPath, src, 0o644); err != nil {
				t.Fatalf("write input: %v", err)
			}
			outPath := strings.TrimSuffix(inPath, ".typ") + ".pdf"

			cmd := exec.Command(typstBin, "compile", "--root", tmp, inPath, outPath)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("typst compile failed: %v\n%s", err, out)
			}
			if fi, statErr := os.Stat(outPath); statErr != nil || fi.Size() == 0 {
				t.Fatalf("expected a non-empty compiled PDF at %s", outPath)
			}
		})
	}
}
