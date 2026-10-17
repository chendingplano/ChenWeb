package docprocessing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetricEnrichmentPromptSeparatesPositionsFromCounts(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "prompts", "prompt-enrich-metrics-v25.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"multi_value_rows_to_split", "20%、80%", "2~3", "percentage positions", "point count"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("prompt missing mixed-unit splitting guidance: %q", want)
		}
	}
}

func timingPointRow(id, value, unit string) map[string]any {
	return map[string]any{
		"candidate_id": id, "metric_name": "计时测试量程取值点",
		"metric_value": value, "unit": unit, "value_range_type": "exact",
		"subject": "计时类测试仪", "source_line_spans": []any{"2"},
		"context": "量程的20%、80%及其中间区段的任意2个～3个数值",
	}
}

func TestEnrichMetricCandidatesSplitsMixedUnits(t *testing.T) {
	count := timingPointRow("7_12", "2~3", "个")
	count["metric_name"], count["value_range_type"] = "中间区段取值点个数", "range"
	count["condition"], count["condition_en"] = "取值区段：量程中间区段", "Region: middle of range"
	ext := &fakeJSONExtractor{outs: []map[string]any{
		{"metrics": []any{timingPointRow("7_12", "20%、80%及中间区段任意2~3个", "个"), timingPointRow("7_13", "3", "次")}},
		{"metrics": []any{timingPointRow("7_12", "20", "%"), timingPointRow("7_12", "80", "%"), count,
			timingPointRow("unrequested", "99", "%")}},
	}}
	p := makeTestMetricsProcessor(ext, 1)
	p.MetricEnrichGroupSize = 12
	metrics, _, _, err := p.enrichMetricCandidates(context.Background(), 371,
		[]metricCandidate{{CandidateID: "7_12", ChunkIndex: 7}, {CandidateID: "7_13", ChunkIndex: 7}},
		metricsBlocksToChunks([]Block{makeMetricsBlock(7, 2)}), "")
	if err != nil || ext.structuredCalledCount != 2 || len(metrics) != 4 {
		t.Fatalf("rows=%v err=%v calls=%d, want four rows after one retry", metrics, err, ext.structuredCalledCount)
	}
	values := map[string]map[string]any{}
	for _, m := range metrics {
		values[asString(m["metric_value"])+metricRowUnit(m)] = m
		if asString(m["candidate_id"]) == "unrequested" || metricValueHoldsSeveralValues(asString(m["metric_value"])) {
			t.Fatalf("invalid replacement: %v", m)
		}
	}
	for _, key := range []string{"20%", "80%", "2~3个", "3次"} {
		if values[key] == nil {
			t.Fatalf("missing %s: %v", key, metrics)
		}
	}
	if m := values["2~3个"]; m["value_min"] != float64(2) || m["value_max"] != float64(3) || asString(m["condition"]) == "" {
		t.Fatalf("count lost range or condition: %v", m)
	}
}

func TestMultiValueRecoveryPreservesOriginalOnInvalidRetry(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  map[string]any
		err  error
	}{
		{"still combined", map[string]any{"metrics": []any{timingPointRow("7_12", "20%、80%及中间区段任意2~3个", "个")}}, nil},
		{"omitted", map[string]any{"metrics": []any{}}, nil},
		{"no evidence", map[string]any{"metrics": []any{map[string]any{"candidate_id": "7_12", "metric_value": "20", "unit": "%"}}}, nil},
		{"empty value", map[string]any{"metrics": []any{timingPointRow("7_12", "", "%")}}, nil},
		{"extractor error", nil, errors.New("retry failed")},
		{"missing count", map[string]any{"metrics": []any{timingPointRow("7_12", "20", "%"), timingPointRow("7_12", "80", "%")}}, nil},
		{"duplicates", map[string]any{"metrics": []any{timingPointRow("7_12", "20", "%"), timingPointRow("7_12", "20", "%")}}, nil},
		{"unrelated evidence", map[string]any{"metrics": []any{
			map[string]any{"candidate_id": "7_12", "metric_name": "Position", "metric_value": "20", "unit": "%", "source_line_spans": []any{"999"}},
			map[string]any{"candidate_id": "7_12", "metric_name": "Position", "metric_value": "80", "unit": "%", "source_line_spans": []any{"999"}},
		}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := timingPointRow("7_12", "20%、80%及中间区段任意2~3个", "个")
			ext := &metricsSeqErrExtractor{outs: []map[string]any{tc.out}, errs: []error{tc.err}}
			p := makeTestMetricsProcessor(ext, 1)
			got := p.recoverMultiValueMetricRows(context.Background(), 371, "call", 7, 1, "source",
				[]metricCandidate{{CandidateID: "7_12", ChunkIndex: 7}}, []map[string]any{original})
			if len(got) != 1 || got[0]["metric_value"] != original["metric_value"] {
				t.Fatalf("retry discarded original: %v", got)
			}
		})
	}
}

func TestMultiValueRecoveryKeepsNonCombinedSiblingAndAllCombinedEvidence(t *testing.T) {
	for _, complete := range []bool{false, true} {
		t.Run(map[bool]string{false: "partial", true: "complete"}[complete], func(t *testing.T) {
			first := timingPointRow("7_12", "20、80", "%")
			second := timingPointRow("7_12", "2、3", "个")
			sibling := timingPointRow("7_12", "5", "次")
			retry := []any{timingPointRow("7_12", "20", "%"), timingPointRow("7_12", "80", "%")}
			if complete {
				retry = append(retry, timingPointRow("7_12", "2", "个"), timingPointRow("7_12", "3", "个"))
			}
			ext := &fakeJSONExtractor{out: map[string]any{"metrics": retry}}
			p := makeTestMetricsProcessor(ext, 1)
			got := p.recoverMultiValueMetricRows(context.Background(), 371, "call", 7, 1, "source",
				[]metricCandidate{{CandidateID: "7_12", ChunkIndex: 7}}, []map[string]any{first, second, sibling})
			wantLen := 3
			if complete {
				wantLen = 5
			}
			if len(got) != wantLen || got[len(got)-1]["metric_value"] != "5" || ext.structuredCalledCount != 1 {
				t.Fatalf("lost original evidence or retried more than once: %v", got)
			}
			if !complete && (got[0]["metric_value"] != first["metric_value"] || got[1]["metric_value"] != second["metric_value"]) {
				t.Fatalf("partial retry discarded combined rows: %v", got)
			}
		})
	}
}

func TestMultiValueRecoveryPreservesSigns(t *testing.T) {
	for _, changedSign := range []bool{false, true} {
		first, second := "-20", "-80"
		if changedSign {
			first, second = "20", "80"
		}
		ext := &fakeJSONExtractor{out: map[string]any{"metrics": []any{timingPointRow("7_12", first, "℃"), timingPointRow("7_12", second, "℃")}}}
		p := makeTestMetricsProcessor(ext, 1)
		got := p.recoverMultiValueMetricRows(context.Background(), 371, "call", 7, 1, "source",
			[]metricCandidate{{CandidateID: "7_12", ChunkIndex: 7}}, []map[string]any{timingPointRow("7_12", "-20、-80", "℃")})
		if changedSign {
			if len(got) != 1 || got[0]["metric_value"] != "-20、-80" {
				t.Fatalf("accepted changed signs: %v", got)
			}
		} else if len(got) != 2 {
			t.Fatalf("rejected valid signed values: %v", got)
		}
	}
}
