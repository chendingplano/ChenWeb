package docprocessing

import (
	"strings"
	"testing"
)

func TestTableMetricHeaderGroupedQuantity(t *testing.T) {
	for _, tc := range []struct {
		name, text, value, unit string
		want                    bool
	}{
		{"grouped tolerance", "supply voltage (240±8%)V", "240±8%", "V", true},
		{"full width parentheses", "supply voltage （240±8%）V", "240±8%", "V", true},
		{"plain scalar", "pressure (24)Pa", "24", "Pa", true},
		{"partial scalar", "pressure (124)Pa", "24", "Pa", false},
		{"partial tolerance", "supply voltage (240±8%)V", "40±8%", "V", false},
		{"missing tolerance", "supply voltage (240±8%)V", "240", "V", false},
		{"unit suffix", "pressure (24)Pa/s", "24", "Pa", false},
		{"different value", "pressure (42)Pa", "24", "Pa", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metric := map[string]any{"metric_value": tc.value, "metric_unit": tc.unit}
			cells, _ := tableMetricHeaderQuantity(TableRow{ID: "h0", Cells: []string{tc.text}, Spans: []int{1}}, metric)
			if (len(cells) != 0) != tc.want {
				t.Fatalf("matched %v, want %v", cells, tc.want)
			}
		})
	}
	metric := map[string]any{"metric_name": "supply voltage", "metric_value": "240±8%", "metric_unit": "V", "source_line_spans": []string{"10"}, "source_table_rows": []TableRowRef{{Line: 10, Rows: []string{"h0"}}}}
	html := `<table><tr><td>26</td><td rowspan="7">Electrical</td><td>Voltage (240±8%)V</td></tr>` + strings.Repeat(`<tr><td>27</td><td>Check wiring</td></tr>`, 6) + `</table>`
	res := buildTableMetricContext(newTableLineIndex([]Line{{LineNo: 10, LineType: "table", Content: html}}), metric)
	if len(res.Refs) != 1 || strings.Join(res.Refs[0].Cells, ",") != "h0:c3" || !strings.Contains(res.Context, "(240±8%)V") {
		t.Fatalf("missing cited header cell and quantity: %+v", res)
	}
}
