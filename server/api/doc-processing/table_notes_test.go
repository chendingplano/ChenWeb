package docprocessing

import (
	"strings"
	"testing"
)

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
