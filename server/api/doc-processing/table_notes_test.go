package docprocessing

import (
	"os"
	"strings"
	"testing"
)

func TestTableCellNotesMixedSuperscriptAndFlattenedLabels(t *testing.T) {
	notes := TableCellNotes("$^a$ 试验水量为名义产水量。b采用名义制热量及进出口5°C温差确定水流量。c最高温度由制造商明示。")
	if len(notes) != 3 || notes[1].Label != "b" || !strings.HasPrefix(notes[1].Text, "采用名义制热量") || strings.Contains(notes[0].Text, "5°C") {
		t.Fatalf("mixed labels were not split: %+v", notes)
	}
	// Latin words and nonsequential variable names are not note boundaries.
	notes = TableCellNotes("$^a$ 压力系数为1。bar为压力单位。x为测量值。")
	if len(notes) != 1 {
		t.Fatalf("ordinary prose split into notes: %+v", notes)
	}
}

func TestRecord825NoteBOverridesBroadRows(t *testing.T) {
	body, err := os.ReadFile("testdata/record825-table-lines.txt")
	if err != nil {
		t.Fatal(err)
	}
	lines, err := ParseInputLines(body)
	if err != nil {
		t.Fatal(err)
	}
	idx := newTableLineIndex(lines)
	g, _ := idx.grid(160)
	for _, mode := range []string{"fresh", "stored_whole_table", "stored_wrong_note"} {
		m := map[string]any{"metric_name": "进出口温差", "metric_value": "5", "unit": "℃", "condition": "水流量：采用名义制热量及进出口5℃温差确定的水流量", "source_line_spans": []any{"160"}}
		if mode != "fresh" {
			ref := TableRowRef{Line: 160, RowHash: map[string]string{}}
			for _, row := range g.Rows {
				ref.Rows = append(ref.Rows, row.ID)
				ref.RowHash[row.ID] = row.Hash
			}
			if mode == "stored_wrong_note" {
				ref.Cells = []string{"r5:c1:na"}
			}
			m["source_table_rows"] = []TableRowRef{ref}
		}
		res := buildTableMetricContext(idx, m)
		if len(res.Refs) != 1 || strings.Join(res.Refs[0].Rows, ",") != "r5" || strings.Join(res.Refs[0].Cells, ",") != "r5:c1:nb" || strings.Contains(res.Context, "[r1]") || strings.Contains(res.Context, "名义产水量") {
			t.Fatalf("%s: note b must replace data rows: %+v", mode, res)
		}
	}
}

func TestTableCellNotesRequireExplicitUnambiguousLabels(t *testing.T) {
	for _, text := range []string{"55 $^c$", "$^a$ 水量 $^a$ 温度", "$^a$", "a ordinary prose b another word"} {
		if notes := TableCellNotes(text); len(notes) != 0 {
			t.Fatalf("unexpected notes for %q: %+v", text, notes)
		}
	}
	notes := TableCellNotes(" $^{a}$ Water volume. $^b$ Defrost conditions.")
	if len(notes) != 2 || notes[1].Label != "b" || notes[1].Text != "Defrost conditions." {
		t.Fatalf("notes: %+v", notes)
	}
}

func TestTableMetricNoteReplacesMatchingHeaderCell(t *testing.T) {
	idx := newTableLineIndex([]Line{{LineNo: 1, LineType: "table", Content: `<table><tr><td>融霜次数</td><td>工况</td></tr><tr><td colspan="2">$^a$ 运行期间水量为10L。 $^b$ 运行期间至少融霜1次。</td></tr></table>`}})
	m := map[string]any{"metric_name": "融霜次数", "metric_value": "1", "unit": "次", "condition": "运行期间", "source_line_spans": []any{"1"}, "source_table_rows": []TableRowRef{{Line: 1, Rows: []string{"r1"}}}}
	res := buildTableMetricContext(idx, m)
	if len(res.Refs) != 1 || strings.Join(res.Refs[0].Cells, ",") != "r1:c1:nb" {
		t.Fatalf("full cell survived narrowing: %+v", res)
	}
	// Evidence in separate notes cannot select either note.
	m["condition"] = "水量"
	res = buildTableMetricContext(idx, m)
	for _, id := range res.Refs[0].Cells {
		if strings.Contains(id, ":n") {
			t.Fatalf("cross-note evidence selected %s", id)
		}
	}
}
