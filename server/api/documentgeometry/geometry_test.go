package documentgeometry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lineFixture = "121\t6\ttable\tfont\t12\t[80,721,890,843]\t<table><tr><td>Type</td><td>Limit</td></tr><tr><td>Machine</td><td>Energy 30</td></tr><tr><td>Biogas</td><td>Volume 50</td></tr></table>\n"

func physicalFixture() PhysicalDocument {
	return PhysicalDocument{Version: 1, Extractor: "pymupdf-tables-v1", PDFHash: "pdf", Tables: []PhysicalTable{
		{Page: 6, Coords: []float64{80, 721, 890, 843}, Rows: []PhysicalRow{
			{Coords: []float64{80, 721, 890, 740}, Cells: []PhysicalCell{{Text: "Type", Coords: []float64{80, 721, 200, 740}}, {Text: "Limit", Coords: []float64{200, 721, 890, 740}}}},
			{Coords: []float64{80, 740, 890, 843}, Cells: []PhysicalCell{{Text: "Machine", Coords: []float64{80, 740, 200, 843}}, {Text: "Energy 30", Coords: []float64{200, 740, 890, 843}}}},
		}},
		{Page: 7, Coords: []float64{80, 100, 890, 200}, Rows: []PhysicalRow{
			{Coords: []float64{80, 100, 890, 120}, Cells: []PhysicalCell{{Text: "Type"}, {Text: "Limit"}}},
			{Coords: []float64{80, 120, 890, 200}, Cells: []PhysicalCell{{Text: "Biogas", Coords: []float64{80, 120, 200, 200}}, {Text: "Volume 50", Coords: []float64{200, 120, 890, 200}}}},
		}},
	}}
}

func TestCanonicalContinuedTableRowsAndCells(t *testing.T) {
	got, err := Build([]byte(lineFixture), physicalFixture())
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tables) != 1 {
		t.Fatalf("tables=%+v", got.Tables)
	}
	r := got.Tables[0].Rows[2]
	if r.ID != "r2" || r.Boxes[0].Page != 7 || r.Hash == "" || r.Cells[1].ID != "r2:c2" || r.Cells[1].Boxes[0].Coords[0] != 200 {
		t.Fatalf("row=%+v", r)
	}
	if len(got.Tables[0].Rows[0].Boxes) != 2 {
		t.Fatal("repeated continuation headers must map to same header")
	}
}

func TestAmbiguousCanonicalRowsOmitted(t *testing.T) {
	line := "1\t6\ttable\tf\t12\t[80,721,890,843]\t<table><tr><td>Type</td><td>Limit</td></tr><tr><td>Machine</td><td>Energy 30</td></tr><tr><td>Machine</td><td>Energy 30</td></tr></table>\n"
	got, err := Build([]byte(line), physicalFixture())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range got.Tables[0].Rows {
		if r.ID != "h0" {
			t.Fatalf("ambiguous data row mapped: %+v", r)
		}
	}
}

func TestMissingPhysicalCompanionDoesNotUseStaleCanonicalFile(t *testing.T) {
	dir := t.TempDir()
	line := filepath.Join(dir, "sample_mineru.txt")
	pdf := filepath.Join(dir, "sample.pdf")
	os.WriteFile(line, []byte(lineFixture), 0600)
	os.WriteFile(pdf, []byte("new PDF"), 0600)
	os.WriteFile(CompanionPath(line), []byte(`{"version":1,"line_sha256":"old","tables":[{"line":121}]}`), 0600)
	got, err := Ensure(line, pdf)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tables) != 0 {
		t.Fatal("stale companion served")
	}
}

func TestRecord416ReportedRowAndCell(t *testing.T) {
	line, err := os.ReadFile("testdata/record416-line.txt")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/record416-physical.json")
	if err != nil {
		t.Fatal(err)
	}
	var physical PhysicalDocument
	if err = json.Unmarshal(raw, &physical); err != nil {
		t.Fatal(err)
	}
	got, err := Build(line, physical)
	if err != nil {
		t.Fatal(err)
	}
	var row *Row
	for i := range got.Tables[0].Rows {
		if got.Tables[0].Rows[i].ID == "r3" {
			row = &got.Tables[0].Rows[i]
		}
	}
	if row == nil || row.Hash != "5364693c56e5" || len(row.Boxes) != 1 || row.Boxes[0].Page != 7 {
		t.Fatalf("incorrect 121#r3: %+v", row)
	}
	if row.Boxes[0].Coords[0] > 120 || row.Boxes[0].Coords[1] < 260 || row.Boxes[0].Coords[3] > 410 {
		t.Fatal("wrong physical row band")
	}
	if len(row.Cells) != 5 || row.Cells[3].ID != "r3:c4" || row.Cells[3].Boxes[0].Coords[0] < 430 {
		t.Fatal("wrong technical-requirements cell")
	}
	if row.Cells[0].Boxes[0].Coords[1] >= row.Boxes[0].Coords[1] {
		t.Fatal("rowspan cell must retain its full physical extent")
	}
}

func TestEnsureInvalidatesPDFLineAndPhysicalGeometryChanges(t *testing.T) {
	dir := t.TempDir()
	line := filepath.Join(dir, "sample_mineru.txt")
	pdf := filepath.Join(dir, "sample.pdf")
	mustWrite := func(p string, b []byte) {
		t.Helper()
		if err := os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(line, []byte(lineFixture))
	mustWrite(pdf, []byte("PDF"))
	physical := physicalFixture()
	physical.PDFHash = hash([]byte("PDF"))
	writePhysical := func() { b, _ := json.Marshal(physical); mustWrite(PhysicalPath(pdf), b) }
	writePhysical()
	initial, err := Ensure(line, pdf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(CompanionPath(line)); err != nil {
		t.Fatal(err)
	}
	initial.Algorithm = "canonical-table-geometry-v1"
	initial.Tables = nil // Simulate a stale mapping with otherwise current source hashes.
	oldCache, _ := json.Marshal(initial)
	mustWrite(CompanionPath(line), oldCache)
	rebuilt, err := Ensure(line, pdf)
	if err != nil || rebuilt.Algorithm != Algorithm || len(rebuilt.Tables) == 0 {
		t.Fatalf("algorithm change did not invalidate cached geometry: %v, %v", rebuilt, err)
	}
	mustWrite(line, []byte(strings.Replace(lineFixture, "121\t", "122\t", 1)))
	changed, err := Ensure(line, pdf)
	if err != nil {
		t.Fatal(err)
	}
	if changed.LineHash == initial.LineHash || changed.Tables[0].Line != 122 {
		t.Fatal("line edit did not invalidate cached geometry")
	}
	physical.Tables[1].Rows[1].Coords[3] = 210
	writePhysical()
	changed, err = Ensure(line, pdf)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Tables[0].Rows[2].Boxes[0].Coords[3] != 210 {
		t.Fatal("physical geometry regeneration did not invalidate cache")
	}
	mustWrite(pdf, []byte("different PDF"))
	stale, err := Ensure(line, pdf)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale.Tables) > 0 {
		t.Fatal("geometry for old PDF was reused")
	}
}

func TestSplitRowKeepsBothPhysicalFragmentsWithoutRepeatedHeader(t *testing.T) {
	line := "121\t6\ttable\tf\t12\t[80,721,890,843]\t<table><tr><td>Type</td><td>Limit</td></tr><tr><td>Biogas</td><td>First part of the requirement Second part of the requirement</td></tr></table>\n"
	physical := physicalFixture()
	physical.Tables[0].Rows[1].Cells[0].Text = "Biogas"
	physical.Tables[0].Rows[1].Cells[1].Text = "First part of the requirement"
	physical.Tables[1].Rows = physical.Tables[1].Rows[1:]
	physical.Tables[1].Rows[0].Cells[1].Text = "Second part of the requirement"
	got, err := Build([]byte(line), physical)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tables[0].Rows[1].Boxes) != 2 {
		t.Fatalf("split row=%+v", got.Tables[0].Rows[1])
	}
}

func TestComparisonSignsAndDecimalPointsRemainSignificant(t *testing.T) {
	for _, tc := range []struct{ source, observed string }{{"≤30", "≥30"}, {"5.5", "55"}} {
		line := "121\t6\ttable\tf\t12\t[80,721,890,843]\t<table><tr><td>Type</td><td>Limit</td></tr><tr><td>Machine</td><td>" + tc.source + "</td></tr></table>\n"
		physical := physicalFixture()
		physical.Tables = physical.Tables[:1]
		physical.Tables[0].Rows[1].Cells[1].Text = tc.observed
		got, err := Build([]byte(line), physical)
		if err != nil {
			t.Fatal(err)
		}
		if len(got.Tables[0].Rows) != 1 {
			t.Fatalf("%s matched %s", tc.source, tc.observed)
		}
	}
}

func TestUnrelatedSamePageTablesCannotContributeRows(t *testing.T) {
	physical := physicalFixture()
	duplicate := physical.Tables[0]
	duplicate.Coords = []float64{80, 850, 890, 990}
	physical.Tables = append([]PhysicalTable{physical.Tables[0], duplicate}, physical.Tables[1])
	got, err := Build([]byte(lineFixture), physical)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tables[0].Rows[1].Boxes) != 1 {
		t.Fatal("unrelated table highlighted")
	}
}

func TestAmbiguousContinuationTablesOmitted(t *testing.T) {
	physical := physicalFixture()
	duplicate := physical.Tables[1]
	duplicate.Coords = []float64{80, 400, 890, 700}
	physical.Tables = append(physical.Tables, duplicate)
	got, err := Build([]byte(lineFixture), physical)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range got.Tables[0].Rows {
		if row.ID == "r2" {
			t.Fatal("ambiguous continuation tables resolved")
		}
	}
}

func TestNewCanonicalCompanionRoundsRowAndCellCoordinates(t *testing.T) {
	physical := physicalFixture()
	physical.Tables[1].Rows[1].Coords = []float64{80.5, 120.49, 890.7, 200.2}
	physical.Tables[1].Rows[1].Cells[1].Coords = []float64{200.5, 120.49, 890.7, 200.2}
	got, err := Build([]byte(lineFixture), physical)
	if err != nil {
		t.Fatal(err)
	}
	row := got.Tables[0].Rows[2]
	for _, box := range []Box{row.Boxes[0], row.Cells[1].Boxes[0]} {
		raw, _ := json.Marshal(box.Coords)
		var ints []int
		if err := json.Unmarshal(raw, &ints); err != nil {
			t.Fatalf("noninteger geometry: %s", raw)
		}
		if ints[0] != int(box.Coords[0]) || ints[1] != 120 || ints[2] != 891 || ints[3] != 200 {
			t.Fatalf("incorrect rounding: %v", ints)
		}
	}
	if row.Boxes[0].Coords[0] != 81 || row.Cells[1].Boxes[0].Coords[0] != 201 {
		t.Fatal("half coordinates must round up")
	}
	if physical.Tables[1].Rows[1].Coords[0] != 80.5 {
		t.Fatal("modified physical input")
	}
}

func TestEnsureLeavesExistingDecimalCompanionUntouched(t *testing.T) {
	dir := t.TempDir()
	line, pdf := filepath.Join(dir, "sample.txt"), filepath.Join(dir, "sample.pdf")
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(line, []byte(lineFixture))
	write(pdf, []byte("PDF"))
	physical := physicalFixture()
	physical.PDFHash = hash([]byte("PDF"))
	raw, _ := json.Marshal(physical)
	write(PhysicalPath(pdf), raw)
	existing, err := Ensure(line, pdf)
	if err != nil {
		t.Fatal(err)
	}
	existing.Tables[0].Rows[0].Boxes[0].Coords[0] = 80.25
	before, _ := json.Marshal(existing)
	write(CompanionPath(line), before)
	got, err := Ensure(line, pdf)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(CompanionPath(line))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || got.Tables[0].Rows[0].Boxes[0].Coords[0] != 80.25 {
		t.Fatal("rewrote existing companion")
	}
}
