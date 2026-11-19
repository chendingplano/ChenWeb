package docprocessing

import "testing"

func TestTableMetricCitationFromRangeOrWrittenCount(t *testing.T) {
	for _, tt := range []struct {
		name, value, unit, first, second string
		wantMatch                        bool
	}{
		{"repeated range unit", "100~200", "mm", "保持载荷离地面100mm~200mm", "继续加载至1.05倍", true},
		{"written count", "2", "处", "电子板不应有多于两处的维修痕迹", "电缆绝缘完好", true},
		{"ambiguous range", "100~200", "mm", "保持载荷离地面100mm~200mm", "保持间距100mm~200mm", false},
		{"different lower endpoint", "100~200", "mm", "保持载荷离地面1100mm~200mm", "电缆绝缘完好", false},
		{"different endpoint units", "100~200", "mm", "保持载荷离地面100cm~200mm", "电缆绝缘完好", false},
		{"unqualified written count", "2", "处", "某两处标识清晰", "电缆绝缘完好", false},
		{"ambiguous written count", "2", "处", "电子板不应有多于两处的维修痕迹", "接线盒裂纹不超过二处", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			lines := []Line{{LineNo: 165, LineType: "table", Content: `<table><tr><th>检查项目</th><th>评估要求</th></tr>` +
				`<tr><td>甲</td><td>` + tt.first + `</td></tr>` +
				`<tr><td>乙</td><td>` + tt.second + `</td></tr></table>`}}
			result := buildTableMetricContext(newTableLineIndex(lines), map[string]any{
				"metric_name": "检查指标", "metric_value": tt.value, "metric_unit": tt.unit,
				"source_line_spans": []string{"165"},
			})
			if tt.wantMatch {
				if len(result.Refs) != 1 || len(result.Refs[0].Rows) != 1 || result.Refs[0].Rows[0] != "r1" {
					t.Fatalf("expected only row r1, got %+v", result.Refs)
				}
			} else if result.Outcome != tableRowsWholeTable {
				t.Fatalf("expected whole table when no unique row matches, got %s with %+v", result.Outcome, result.Refs)
			}
		})
	}
}
