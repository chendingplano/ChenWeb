package kbhandler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMetricReviewSpansFromText(t *testing.T) {
	got := metricReviewSpansFromText("L12, 13:15; 20-21, invalid, 25:22")
	want := []string{"12", "13:15", "20:21"}
	if len(got) != len(want) {
		t.Fatalf("spans = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("spans = %v, want %v", got, want)
		}
	}
}

func TestBuildMetricReviewInput_FormatsLinesAndMetrics(t *testing.T) {
	h := metricReviewDocHeader{RecordID: 416, Title: "农村生活垃圾分类处理规范", DocNo: "DB33/T"}
	lines := []rawLine{
		{LineNumber: 50, LineType: "paragraph", Content: "农村生活垃圾分为四大类"},
		{LineNumber: 126, LineType: "table", Content: "<table>...</table>"},
	}
	metrics := []metricReviewInputMetric{{MetricID: "416_mtc_2", MetricName: "分类类别数", MetricValue: "4"}}

	text, err := buildMetricReviewInput(h, "zh-cn", lines, metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{
		"record_id: 416",
		"title: 农村生活垃圾分类处理规范",
		"output_language: zh-cn (Simplified Chinese)\n",
		"L50\tparagraph\t农村生活垃圾分为四大类\n",
		"L126\ttable\t<table>...</table>\n",
		"METRICS\n",
		`"metric_id": "416_mtc_2"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("input missing %q\n---\n%s", want, text)
		}
	}
	if strings.Contains(text, "value_min") {
		t.Errorf("null value_min should be omitted")
	}
}

func TestBuildMetricReviewInput_TooLarge(t *testing.T) {
	big := strings.Repeat("字", metricReviewMaxInputChars)
	_, err := buildMetricReviewInput(metricReviewDocHeader{RecordID: 1}, "en",
		[]rawLine{{LineNumber: 1, LineType: "paragraph", Content: big}},
		[]metricReviewInputMetric{{MetricID: "1_mtc_1"}})
	if err == nil || !strings.Contains(err.Error(), "document too large") {
		t.Fatalf("want document-too-large error, got %v", err)
	}
}

func TestFinalizeMetricReview_TallyAndFiltering(t *testing.T) {
	metrics := []metricReviewInputMetric{
		{MetricID: "m1", MetricName: "a", SourceLineSpans: json.RawMessage(`["50"]`)},
		{MetricID: "m2", MetricName: "b"},
		{MetricID: "m3", MetricName: "c"},
		{MetricID: "m4", MetricName: "d"},
		{MetricID: "m5", MetricName: "e"},
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{
		"summary": "ok",
		"tally": {"stored": 999},
		"missed_metrics": [{"lines": 123, "name": "x", "value": 60, "severity": "HIGH"}],
		"non_metrics": [
			{"metric_ids": ["m1", "ghost"], "category": "not_metric", "reason": "slogan"},
			{"metric_ids": ["m2"], "category": "duplicate", "duplicate_of": "m3"},
			{"metric_ids": ["m1", "m4"], "category": "formula_input"},
			{"metric_ids": ["ghost2"], "category": "not_metric"},
			{"metric_ids": ["m5"], "category": "weird", "duplicate_of": "m1"}
		],
		"attribute_issues": [
			{"metric_ids": ["m3", "ghost"], "field": "condition", "severity": "bogus"},
			{"metric_ids": ["ghost3"], "field": "unit"}
		],
		"recommendations": ["fix merge", "  "]
	}`), &payload); err != nil {
		t.Fatal(err)
	}

	report, dropped, err := finalizeMetricReview(payload, metrics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := metricReviewTally{Stored: 5, Kept: 1, NotMetric: 2, Duplicate: 1, FormulaInput: 1, Missed: 1}
	if report.Tally != want {
		t.Errorf("tally = %+v, want %+v", report.Tally, want)
	}
	if got := report.MissedMetrics[0].SourceLineSpans; len(got) != 1 || got[0] != "123" {
		t.Errorf("missed source spans = %v", got)
	}
	if got := report.Metrics[0].SourceLineSpans; len(got) != 1 || got[0] != "50" {
		t.Errorf("metric snapshot source spans = %v", got)
	}
	if strings.Join(dropped, ",") != "ghost,ghost2,ghost3" {
		t.Errorf("dropped = %v", dropped)
	}
	if len(report.NonMetrics) != 4 {
		t.Fatalf("non_metrics len = %d, want 4 (ghost-only entry removed)", len(report.NonMetrics))
	}
	if got := report.NonMetrics[0].MetricIDs; len(got) != 1 || got[0] != "m1" {
		t.Errorf("unknown id not filtered: %v", got)
	}
	if report.NonMetrics[1].DuplicateOf != "m3" {
		t.Errorf("duplicate_of lost: %q", report.NonMetrics[1].DuplicateOf)
	}
	if nm := report.NonMetrics[3]; nm.Category != "not_metric" || nm.DuplicateOf != "" {
		t.Errorf("category/duplicate_of not normalised: %+v", nm)
	}
	if len(report.AttributeIssues) != 1 || report.AttributeIssues[0].Severity != "medium" {
		t.Errorf("attribute issues = %+v", report.AttributeIssues)
	}
	if m := report.MissedMetrics[0]; m.Lines != "123" || m.Value != "60" || m.Severity != "high" {
		t.Errorf("missed metric not normalised: %+v", m)
	}
	if len(report.Recommendations) != 1 {
		t.Errorf("blank recommendation kept: %v", report.Recommendations)
	}
	if len(report.Metrics) != 5 || report.Metrics[0].Lines != "50" {
		t.Errorf("snapshot = %+v", report.Metrics)
	}
}

func TestFinalizeMetricReview_SchemaMismatch(t *testing.T) {
	_, _, err := finalizeMetricReview(map[string]any{"non_metrics": "not-a-list"}, nil)
	if err == nil {
		t.Fatal("want schema error")
	}
}

func TestShouldStartMetricReview(t *testing.T) {
	cases := []struct {
		name   string
		latest *metricReviewRow
		force  bool
		want   bool
	}{
		{"none", nil, false, true},
		{"done no force", &metricReviewRow{Status: metricReviewStatusDone}, false, false},
		{"done force", &metricReviewRow{Status: metricReviewStatusDone}, true, true},
		{"running force", &metricReviewRow{Status: metricReviewStatusRunning}, true, false},
		{"failed no force", &metricReviewRow{Status: metricReviewStatusFailed}, false, true},
	}
	for _, tc := range cases {
		if got := shouldStartMetricReview(tc.latest, tc.force); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestApplyStaleRunningStatus(t *testing.T) {
	now := time.Now()
	fresh := &metricReviewRow{Status: metricReviewStatusRunning, CreatedAt: now.Add(-time.Minute)}
	applyStaleRunningStatus(fresh, now)
	if fresh.Status != metricReviewStatusRunning {
		t.Errorf("fresh run marked %s", fresh.Status)
	}
	stale := &metricReviewRow{Status: metricReviewStatusRunning, CreatedAt: now.Add(-metricReviewRunTimeout - time.Minute)}
	applyStaleRunningStatus(stale, now)
	if stale.Status != metricReviewStatusFailed || !strings.Contains(stale.ErrorMsg, "interrupted") {
		t.Errorf("stale run = %+v", stale)
	}
	// A stale running row must not block a new review.
	if !shouldStartMetricReview(stale, false) {
		t.Error("stale run should allow a new review")
	}
}

func TestNormalizeMetricReviewLang(t *testing.T) {
	for in, want := range map[string]string{"": "en", "en": "en", "EN": "en", "zh-cn": "zh-cn", "zh_CN": "zh-cn", "zh": "zh-cn"} {
		got, ok := normalizeMetricReviewLang(in)
		if !ok || got != want {
			t.Errorf("normalizeMetricReviewLang(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	if _, ok := normalizeMetricReviewLang("fr"); ok {
		t.Errorf("fr should be unsupported")
	}
}

func TestMetricReviewTranslationStrings_CollectAndApply(t *testing.T) {
	r := metricReviewReport{
		Summary:       "Good extraction.",
		Tally:         metricReviewTally{Stored: 3, Kept: 2, Missed: 1},
		MissedMetrics: []metricReviewMissed{{Lines: "153", Name: "bottle volume", Value: "500", Unit: "mL", Reason: "missed", Severity: "medium"}},
		NonMetrics:    []metricReviewNonMetric{{MetricIDs: []string{"m1"}, Category: "not_metric", Reason: "descriptor"}},
		AttributeIssues: []metricReviewAttributeIssue{{MetricIDs: []string{"m2"}, Field: "value_range_type", Stored: "range", Suggested: "lower_bound", Reason: "wording", Severity: "high"},
			{MetricIDs: []string{"m3"}, Field: "metric_value", Stored: "Comparator included in value", Suggested: "30", Reason: "bare number", Severity: "medium"}},
		Recommendations: []string{"do x", ""},
	}
	got := collectMetricReviewStrings(&r)
	want := []string{"summary", "missed_metrics.0.name", "missed_metrics.0.reason", "non_metrics.0.reason",
		"attribute_issues.0.reason", "attribute_issues.1.stored", "attribute_issues.1.reason", "recommendations.0"}
	if len(got) != len(want) {
		t.Fatalf("collected %d strings, want %d: %v", len(got), len(want), got)
	}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q", k)
		}
	}

	n := applyMetricReviewStrings(&r, map[string]any{
		"summary":                   "提取良好。",
		"missed_metrics.0.reason":   "遗漏",
		"attribute_issues.1.stored": "值中包含比较符",
		"non_metrics.0.reason":      "",  // empty: keep source
		"recommendations.0":         42,  // non-string: keep source
		"unknown.key":               "x", // ignored
	})
	if n != 3 {
		t.Errorf("applied = %d, want 3", n)
	}
	if r.Summary != "提取良好。" || r.MissedMetrics[0].Reason != "遗漏" || r.AttributeIssues[1].Stored != "值中包含比较符" {
		t.Errorf("translation not applied: %+v", r)
	}
	if r.NonMetrics[0].Reason != "descriptor" || r.Recommendations[0] != "do x" || r.MissedMetrics[0].Name != "bottle volume" {
		t.Errorf("untranslated fields should keep source text: %+v", r)
	}
	if r.Tally.Stored != 3 || r.AttributeIssues[0].Stored != "range" || r.AttributeIssues[0].Suggested != "lower_bound" || r.AttributeIssues[1].Suggested != "30" || r.MissedMetrics[0].Lines != "153" || r.AttributeIssues[0].Severity != "high" {
		t.Errorf("non-prose fields must be unchanged: %+v", r)
	}
}

func TestListMetricReviewModels_OnlyLLM(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".models.toml")
	content := `
[zeta-llm]
model_type = 'llm'
model_name = 'z'

[bge-embed]
model_type = 'embedding'
model_name = 'bge'

[alpha-llm]
model_type = 'llm'
model_name = 'a'

[jev]
model_type = 'decision-model'
model_name = 'jev'

[model-api-keys]
[[model-api-keys.x]]
api_key = 'k'
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MODEL_DEF_FILE", path)
	got, err := listMetricReviewModels()
	if err != nil {
		t.Fatalf("listMetricReviewModels: %v", err)
	}
	want := []string{"alpha-llm", "zeta-llm"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("models = %v, want %v", got, want)
	}
}
