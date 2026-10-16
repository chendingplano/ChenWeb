package docprocessing

import (
	"slices"
	"testing"
)

func TestNormalizeMetricValueNotation(t *testing.T) {
	cases := []struct {
		name                   string
		value, unit, rangeType string
		wantValue, wantRange   string
		wantMin, wantMax       any
		wantStrict             bool
	}{
		{name: "tolerance stored as exact", value: "±2", unit: "mm", rangeType: "exact", wantValue: "±2", wantRange: "upper_bound"},
		{name: "tolerance stored as range", value: "±1.5", unit: "%", rangeType: "range", wantValue: "±1.5", wantRange: "upper_bound"},
		{name: "tolerance with unit inside", value: "±0.5%", unit: "%", rangeType: "exact", wantValue: "±0.5", wantRange: "upper_bound"},
		{name: "centre with tolerance stays range", value: "20±5", unit: "℃", rangeType: "range", wantValue: "20±5", wantRange: "range", wantMin: float64(15), wantMax: float64(25)},
		{name: "unit repeated in a range", value: "5 s~25 s", unit: "s", rangeType: "range", wantValue: "5~25", wantRange: "range", wantMin: float64(5), wantMax: float64(25)},
		{name: "full-width tilde and spaced thousands", value: "600 mm～1 600 mm", unit: "mm", rangeType: "range", wantValue: "600~1600", wantRange: "range", wantMin: float64(600), wantMax: float64(1600)},
		{name: "negative lower endpoint", value: "-100 mm~300 mm", unit: "mm", rangeType: "range", wantValue: "-100~300", wantRange: "range", wantMin: float64(-100), wantMax: float64(300)},
		{name: "unit after a single number", value: "1次", unit: "次", rangeType: "exact", wantValue: "1", wantRange: "exact"},
		{name: "composite unit is not stripped partially", value: "120次/min", unit: "次", rangeType: "exact", wantValue: "120次/min", wantRange: "exact"},
		{name: "composite unit stripped whole", value: "120次/min", unit: "次/min", rangeType: "exact", wantValue: "120", wantRange: "exact"},
		{name: "leading comparator with unit", value: "≥500 mm", unit: "mm", rangeType: "lower_bound", wantValue: "500", wantRange: "lower_bound"},
		{name: "comparator sets range type of exact", value: "≤20", unit: "s", rangeType: "exact", wantValue: "20", wantRange: "upper_bound"},
		{name: "strict comparator adds tag", value: "大于5", unit: "", rangeType: "lower_bound", wantValue: "5", wantRange: "lower_bound", wantStrict: true},
		{name: "plain interval gets endpoints", value: "100~9999", unit: "mL", rangeType: "range", wantValue: "100~9999", wantRange: "range", wantMin: float64(100), wantMax: float64(9999)},
		{name: "multi-value row unchanged", value: "男300 mm、女250 mm", unit: "mm", rangeType: "exact", wantValue: "男300 mm、女250 mm", wantRange: "exact"},
		{name: "fraction unchanged", value: "1/10", unit: "", rangeType: "upper_bound", wantValue: "1/10", wantRange: "upper_bound"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := map[string]any{"metric_value": c.value, "unit": c.unit, "value_range_type": c.rangeType}
			normalizeMetricValueNotation([]map[string]any{m})
			if m["metric_value"] != c.wantValue || m["value_range_type"] != c.wantRange {
				t.Fatalf("got value=%q range=%q, want %q %q", m["metric_value"], m["value_range_type"], c.wantValue, c.wantRange)
			}
			if m["value_min"] != c.wantMin || m["value_max"] != c.wantMax {
				t.Fatalf("got min=%#v max=%#v, want %#v %#v", m["value_min"], m["value_max"], c.wantMin, c.wantMax)
			}
			if got := slices.Contains(toStringSlice(m["reasoning_tags"]), "strict_bound"); got != c.wantStrict {
				t.Fatalf("strict_bound tag=%v, want %v", got, c.wantStrict)
			}
		})
	}
}

func TestNormalizeMetricValueNotationKeepsModelEndpoints(t *testing.T) {
	m := map[string]any{"metric_value": "0.5~20.0", "unit": "m", "value_range_type": "range", "value_min": float64(0.5), "value_max": float64(20)}
	normalizeMetricValueNotation([]map[string]any{m})
	if m["value_min"] != float64(0.5) || m["value_max"] != float64(20) {
		t.Fatalf("endpoints changed: %#v %#v", m["value_min"], m["value_max"])
	}
}

func TestMultiValueMetricRows(t *testing.T) {
	rows := []map[string]any{
		{"metric_name": "台阶高度", "metric_value": "男300 mm、女250 mm"},
		{"metric_name": "允许误差", "metric_value": "≤50 kg:±0.1 kg；>50 kg:±0.2 kg"},
		{"metric_name": "主尺检验点", "metric_value": "10、30、50"},
		{"metric_name": "有效量程", "metric_value": "600~1600"},
		{"metric_name": "误差比例", "metric_value": "1/10"},
		{"metric_name": "量程", "metric_value": "0次/min~300次/min"},
	}
	got := multiValueMetricRows(rows)
	var names []string
	for _, r := range got {
		names = append(names, r["metric_name"].(string))
	}
	if !slices.Equal(names, []string{"台阶高度", "允许误差", "主尺检验点"}) {
		t.Fatalf("multi-value rows = %v", names)
	}
}

func TestToOptionalFloat(t *testing.T) {
	for _, c := range []struct {
		in   any
		want any
	}{{nil, nil}, {"", nil}, {"abc", nil}, {float64(0), float64(0)}, {"1.5", float64(1.5)}, {int(3), float64(3)}} {
		if got := toOptionalFloat(c.in); got != c.want {
			t.Fatalf("toOptionalFloat(%#v)=%#v, want %#v", c.in, got, c.want)
		}
	}
}

func TestNormalizeMetricListKeepsConditionAndEndpoints(t *testing.T) {
	got := normalizeMetricList([]any{map[string]any{
		"metric_name": "有效量程", "metric_value": "600~1600", "value_range_type": "range",
		"condition": " 适用人群：幼儿 ", "condition_en": "Population: preschool children",
		"value_min": float64(600), "value_max": float64(1600),
	}, map[string]any{"metric_name": "分度值", "metric_value": "1"}})
	if got[0]["condition"] != "适用人群：幼儿" || got[0]["condition_en"] != "Population: preschool children" {
		t.Fatalf("condition=%#v condition_en=%#v", got[0]["condition"], got[0]["condition_en"])
	}
	if got[0]["value_min"] != float64(600) || got[0]["value_max"] != float64(1600) {
		t.Fatalf("endpoints=%#v %#v", got[0]["value_min"], got[0]["value_max"])
	}
	if got[1]["value_min"] != nil || got[1]["value_max"] != nil {
		t.Fatalf("missing endpoints must stay nil, got %#v %#v", got[1]["value_min"], got[1]["value_max"])
	}
}

func TestDedupeFinalMetricRowsKeepsDifferentConditions(t *testing.T) {
	row := func(cond string) map[string]any {
		return map[string]any{"metric_name": "分度值", "subject": "握力测试仪(计)", "unit": "N", "metric_value": "1",
			"source_line_spans": []string{"116"}, "condition": cond}
	}
	got := dedupeFinalMetricRows([]map[string]any{row("适用人群：幼儿"), row("适用人群：成年人"), row("适用人群：幼儿")})
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 (one per condition)", len(got))
	}
}

func TestMetricIdentityRequiresSameCondition(t *testing.T) {
	a := map[string]any{"metric_name": "台阶高度", "metric_subject": "台阶试验测试仪(计)", "metric_unit": "mm",
		"metric_value": "300", "keyword_concept_id": "kwc:step_height", "source_line_spans": []string{"113"}, "condition": "性别：男"}
	b := cloneMetricMap(a)
	b["metric_value"], b["condition"] = "250", "性别：女"
	if metricsIdentical(a, b) {
		t.Fatal("same concept, different condition must not be identical")
	}
	c := cloneMetricMap(a)
	c["condition"] = " 性别： 男 "
	if !metricsIdentical(a, c) {
		t.Fatal("conditions differing only in whitespace must be identical")
	}

	withCond := map[string]any{"metric_name": "有效量程", "metric_subject": "身高测试仪(计)", "metric_unit": "mm", "condition": "适用人群：幼儿"}
	noCond := map[string]any{"metric_name": "有效量程", "metric_subject": "身高测试仪(计)", "metric_unit": "mm"}
	if metricStaticMatch(withCond, noCond) {
		t.Fatal("an empty and a non-empty condition must not static-match")
	}
	if !metricStaticMatch(withCond, cloneMetricMap(withCond)) {
		t.Fatal("same condition must still static-match")
	}
	if _, ok := metricMergeComparedFields(withCond)["condition"]; !ok {
		t.Fatal("condition missing from merge compared fields")
	}
}

func TestDatasetRecordTagIsSetAside(t *testing.T) {
	kept, dropped := dropRowsTaggedWithDropReason([]map[string]any{
		{"metric_name": "年龄", "reasoning_tags": []string{"dataset_record"}},
		{"metric_name": "有效量程", "reasoning_tags": []string{"has_unit"}},
	})
	if len(kept) != 1 || len(dropped) != 1 || dropped[0]["metric_name"] != "年龄" {
		t.Fatalf("kept=%v dropped=%v", kept, dropped)
	}
}
