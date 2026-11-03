package documentgeometry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
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
	// Row IDs come from ParseTableGrid: a cache built before its numbering changed
	// (record 753 表2, rowspan header) must not keep serving the old IDs.
	rebuilt.TableGridVersion = docprocessing.TableGridVersion - 1
	rebuilt.Tables = nil
	oldCache, _ = json.Marshal(rebuilt)
	mustWrite(CompanionPath(line), oldCache)
	rebuilt, err = Ensure(line, pdf)
	if err != nil || rebuilt.TableGridVersion != docprocessing.TableGridVersion || len(rebuilt.Tables) == 0 {
		t.Fatalf("table grid version change did not invalidate cached geometry: %v, %v", rebuilt, err)
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

func TestGarbledIdentifierCellsMatchOnReadableText(t *testing.T) {
	line := "145\t10\ttable\tunknown-font\t12\t[115,571,895,916]\t<table><tr><td>内部标识符</td><td>中文名称</td></tr><tr><td>1010010</td><td>姓名受检者正式登记</td></tr><tr><td>1010021</td><td>身份证件类型</td></tr></table>\n"
	physical := PhysicalDocument{Version: 1, Extractor: "pymupdf-tables-v1", PDFHash: "pdf", Tables: []PhysicalTable{{Page: 10, Coords: []float64{115, 571, 895, 916}, Rows: []PhysicalRow{
		{Coords: []float64{115, 571, 895, 600}, Cells: []PhysicalCell{{Text: "内部 标识符", Coords: []float64{115, 571, 300, 600}}, {Text: "中文名称", Coords: []float64{300, 571, 895, 600}}}},
		{Coords: []float64{115, 600, 895, 650}, Cells: []PhysicalCell{{Text: "!+!++!+", Coords: []float64{115, 600, 300, 650}}, {Text: "姓名受检者正式登记", Coords: []float64{300, 600, 895, 650}}}},
		{Coords: []float64{115, 650, 895, 700}, Cells: []PhysicalCell{{Text: "!+!++\"!", Coords: []float64{115, 650, 300, 700}}, {Text: "身 份 证 件 类型", Coords: []float64{300, 650, 895, 700}}}},
	}}}}
	got, err := Build([]byte(line), physical)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tables) != 1 || len(got.Tables[0].Rows) != 3 || got.Tables[0].Rows[1].ID != "r1" || got.Tables[0].Rows[2].ID != "r2" {
		t.Fatalf("rows=%+v", got.Tables)
	}
}

// Rows that differ only in garbled digits tie on their Chinese text; table order decides them.
func TestReadableTiesResolvedByRowOrder(t *testing.T) {
	line := "145\t10\ttable\tunknown-font\t12\t[115,571,895,916]\t<table><tr><td>内部标识符</td><td>中文名称</td><td>说明</td></tr>" +
		"<tr><td>3070030</td><td>超敏C反应蛋白</td><td>受检者血清中超敏C反应蛋白浓度的检测值</td></tr>" +
		"<tr><td>3071010</td><td>癌抗原125</td><td>受检者血清中癌抗原125浓度的检测值</td></tr>" +
		"<tr><td>3071020</td><td>癌抗原153</td><td>受检者血清中癌抗原153浓度的检测值</td></tr>" +
		"<tr><td>3071030</td><td>糖链抗原199</td><td>受检者血清中糖链抗原199浓度的检测值</td></tr></table>\n"
	row := func(y float64, cells ...string) PhysicalRow {
		r := PhysicalRow{Coords: []float64{115, y, 895, y + 20}}
		for _, c := range cells {
			r.Cells = append(r.Cells, PhysicalCell{Text: c})
		}
		return r
	}
	physical := PhysicalDocument{Version: 1, Extractor: "pymupdf-tables-v1", PDFHash: "pdf", Tables: []PhysicalTable{{Page: 10, Coords: []float64{115, 571, 895, 916}, Rows: []PhysicalRow{
		row(571, "内部 标识符", "中文名称", "说明"),
		row(600, "#+)+#+", "超敏 反应蛋白", "受检者血清中超敏 反应蛋白浓度的检测值"),
		row(620, "#+(!+!+", "癌抗原 !\"%", "受检者血清中癌抗原 浓度的检测值"),
		row(640, "#+(!+\"+", "癌抗原 !%#", "受检者血清中癌抗原 浓度的检测值"),
		row(660, "#+(!+#+", "糖链抗原 !**", "受检者血清中糖链抗原 浓度的检测值"),
	}}}}
	got, err := Build([]byte(line), physical)
	if err != nil {
		t.Fatal(err)
	}
	tops := map[string]float64{}
	for _, r := range got.Tables[0].Rows {
		tops[r.ID] = r.Boxes[0].Coords[1]
	}
	if tops["r2"] != 620 || tops["r3"] != 640 || tops["r1"] != 600 || tops["r4"] != 660 {
		t.Fatalf("row tops=%v", tops)
	}
}

// A glyph the PDF font maps to the private use area (啰 read as U+E3FE) stands for one character.
func TestPrivateUseGlyphMatchesOneChineseCharacter(t *testing.T) {
	line := "145\t10\ttable\tunknown-font\t12\t[115,571,895,916]\t<table><tr><td>内部标识符</td><td>中文名称</td></tr><tr><td>2070023</td><td>肺部啰音标志</td></tr></table>\n"
	physical := PhysicalDocument{Version: 1, Extractor: "pymupdf-tables-v1", PDFHash: "pdf", Tables: []PhysicalTable{{Page: 10, Coords: []float64{115, 571, 895, 916}, Rows: []PhysicalRow{
		{Coords: []float64{115, 571, 895, 600}, Cells: []PhysicalCell{{Text: "内部 标识符"}, {Text: "中文名称"}}},
		{Coords: []float64{115, 600, 895, 650}, Cells: []PhysicalCell{{Text: "\"+(++\"#"}, {Text: "肺 部  音 标志"}}},
	}}}}
	got, err := Build([]byte(line), physical)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Tables[0].Rows) != 2 || got.Tables[0].Rows[1].ID != "r1" {
		t.Fatalf("rows=%+v", got.Tables[0].Rows)
	}
}

// stackedPhysical is GB 9448 表 1 in miniature: the 手工电弧焊 row stacks three values per
// cell with no rule between them, so the physical row carries three sub-rows.
func stackedPhysical() PhysicalDocument {
	cell := func(text string, x0, y0, x1, y1 float64) PhysicalCell {
		return PhysicalCell{Text: text, Coords: []float64{x0, y0, x1, y1}}
	}
	name := cell("手工电弧焊", 100, 200, 300, 260)
	sub := func(y0, y1 float64, size, shade string) PhysicalRow {
		return PhysicalRow{Coords: []float64{100, y0, 900, y1}, Cells: []PhysicalCell{name, cell(size, 300, y0, 600, y1), cell(shade, 600, y0, 900, y1)}}
	}
	return PhysicalDocument{Version: 1, Extractor: "pymupdf-tables-v2", PDFHash: "pdf", Tables: []PhysicalTable{{Page: 12, Coords: []float64{100, 170, 900, 290}, Rows: []PhysicalRow{
		{Coords: []float64{100, 170, 900, 200}, Cells: []PhysicalCell{cell("焊接方法", 100, 170, 300, 200), cell("焊条尺寸 mm", 300, 170, 600, 200), cell("最低遮光号", 600, 170, 900, 200)}},
		{Coords: []float64{100, 200, 900, 260}, Cells: []PhysicalCell{name, cell("<2.5 2.5~<4 >4", 300, 200, 600, 260), cell("7 8 10", 600, 200, 900, 260)},
			SubRows: []PhysicalRow{sub(200, 220, "<2.5", "7"), sub(220, 240, "2.5~<4", "8"), sub(240, 260, ">4", "10")}},
		{Coords: []float64{100, 260, 900, 290}, Cells: []PhysicalCell{cell("碳弧焊", 100, 260, 300, 290), cell("—", 300, 260, 600, 290), cell("14", 600, 260, 900, 290)}},
	}}}}
}

func TestSplitCanonicalRowsMatchPhysicalSubRows(t *testing.T) {
	line := "123\t12\ttable\tunknown-font\t12\t[100,170,900,290]\t<table><tr><td>焊接方法</td><td>焊条尺寸mm</td><td>最低遮光号</td></tr>" +
		"<tr><td rowspan=\"3\">手工电弧焊</td><td>&lt;2.5</td><td>7</td></tr><tr><td>2.5~&lt;4</td><td>8</td></tr><tr><td>&gt;4</td><td>10</td></tr>" +
		"<tr><td>碳弧焊</td><td>—</td><td>14</td></tr></table>\n"
	got, err := Build([]byte(line), stackedPhysical())
	if err != nil {
		t.Fatal(err)
	}
	rows := got.Tables[0].Rows
	if len(rows) != 5 {
		t.Fatalf("rows=%+v", rows)
	}
	for i, top := range []float64{200, 220, 240} {
		r := rows[i+1]
		if len(r.Boxes) != 1 || r.Boxes[0].Coords[1] != top || r.Cells[1].Boxes[0].Coords[1] != top {
			t.Fatalf("%s must get only its sub-row box at y=%v: %+v", r.ID, top, r)
		}
	}
}

func TestGluedCanonicalRowKeepsPhysicalBand(t *testing.T) {
	line := "123\t12\ttable\tunknown-font\t12\t[100,170,900,290]\t<table><tr><td>焊接方法</td><td>焊条尺寸mm</td><td>最低遮光号</td></tr>" +
		"<tr><td>手工电弧焊</td><td>&lt;2.52.5~&lt;4&gt;4</td><td>7810</td></tr><tr><td>碳弧焊</td><td>—</td><td>14</td></tr></table>\n"
	got, err := Build([]byte(line), stackedPhysical())
	if err != nil {
		t.Fatal(err)
	}
	r := got.Tables[0].Rows[1]
	if r.ID != "r1" || len(r.Boxes) != 1 || r.Boxes[0].Coords[1] != 200 || r.Boxes[0].Coords[3] != 260 {
		t.Fatalf("glued row must keep the band box: %+v", r)
	}
}
