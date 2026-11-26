package docprocessing

import (
	"strings"
	"testing"
)

func TestTableMetricCombinedNameLabelInAnyColumn(t *testing.T) {
	const header = `<table><tr><td>Index</td><td>Key</td><td>Label</td><td>Type</td><td>Maximum length</td></tr>`
	const first = `<tr><td>1</td><td>alpha</td><td>Account name</td><td>String</td><td>24</td></tr>`
	for _, tc := range []struct{ name, rows, value, want string }{
		{"label in third column", first + `<tr><td>2</td><td>beta</td><td>Display name</td><td>String</td><td>24</td></tr>`, "24", "r1:c5"},
		{"duplicate label and value", first + first, "24", ""},
		{"wrong value", first, "42", ""},
		{"partial number", first, "4", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			grid, err := ParseTableGrid(header + tc.rows + `</table>`)
			if err != nil {
				t.Fatal(err)
			}
			metric := map[string]any{"metric_name": "Account nameMaximum length", "metric_value": tc.value, "metric_unit": "characters"}
			_, cell := matchTableMetricCombinedName(grid, metric)
			if cell != tc.want {
				t.Fatalf("cell = %q, want %q", cell, tc.want)
			}
		})
	}
	// Repeated values in a large table must still produce one exact row and cell.
	metric := map[string]any{"metric_name": "Account nameMaximum length", "metric_value": "24", "source_line_spans": []string{"10"}}
	html := header + first + strings.Repeat(`<tr><td>2</td><td>other</td><td>Other field</td><td>String</td><td>24</td></tr>`, 8) + `</table>`
	res := buildTableMetricContext(newTableLineIndex([]Line{{LineNo: 10, LineType: "table", Content: html}}), metric)
	if len(res.Refs) != 1 || strings.Join(res.Refs[0].Cells, ",") != "r1:c5" || res.Refs[0].RowHash["r1"] == "" {
		t.Fatalf("missing exact hashed citation: %+v", res)
	}
}
