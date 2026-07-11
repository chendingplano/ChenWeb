package docprocessing

import "testing"

func TestMetricsIdentical_SameFieldsMatch(t *testing.T) {
	a := map[string]any{
		"metric_name": "Latency", "metric_subject": "gateway", "metric_unit": "ms",
		"metric_value": "200", "source_line_spans": []any{float64(2)},
	}
	b := map[string]any{
		"metric_name": "Latency", "metric_subject": "gateway", "metric_unit": "ms",
		"metric_value": "200", "source_line_spans": []any{float64(2)},
	}
	if !metricsIdentical(a, b) {
		t.Fatalf("expected identical")
	}
}

func TestMetricsIdentical_DifferentValueNotMatch(t *testing.T) {
	a := map[string]any{
		"metric_name": "Latency", "metric_subject": "gateway", "metric_unit": "ms",
		"metric_value": "200", "source_line_spans": []any{float64(2)},
	}
	b := map[string]any{
		"metric_name": "Latency", "metric_subject": "gateway", "metric_unit": "ms",
		"metric_value": "300", "source_line_spans": []any{float64(2)},
	}
	if metricsIdentical(a, b) {
		t.Fatalf("expected not identical")
	}
}

func TestMetricLineSpansOverlap_True(t *testing.T) {
	a := map[string]any{"source_line_spans": []any{float64(2), float64(3)}}
	b := map[string]any{"source_line_spans": []any{"3:4"}}
	if !metricLineSpansOverlap(a, b) {
		t.Fatalf("expected overlap (both cover line 3)")
	}
}

func TestMetricLineSpansOverlap_False(t *testing.T) {
	a := map[string]any{"source_line_spans": []any{float64(2)}}
	b := map[string]any{"source_line_spans": []any{float64(9)}}
	if metricLineSpansOverlap(a, b) {
		t.Fatalf("expected no overlap")
	}
}

// TestComputeMetricGroups_TransitiveChain regression-tests the single-hop bug
// the ADR calls out: A overlaps B, B overlaps C, but A and C do not directly
// overlap. All three must land in one group.
func TestComputeMetricGroups_TransitiveChain(t *testing.T) {
	metrics := []map[string]any{
		{"source_line_spans": []any{float64(1), float64(2)}},  // A: lines 1-2
		{"source_line_spans": []any{float64(2), float64(3)}},  // B: lines 2-3 (overlaps A)
		{"source_line_spans": []any{float64(3), float64(4)}},  // C: lines 3-4 (overlaps B, not A)
		{"source_line_spans": []any{float64(100)}},            // D: unrelated
	}
	groups := computeMetricGroups(metrics)
	if len(groups) != 2 {
		t.Fatalf("groups=%d, want 2 (one of size 3, one of size 1); got %+v", len(groups), groups)
	}
	var sizes []int
	for _, g := range groups {
		sizes = append(sizes, len(g))
	}
	found3, found1 := false, false
	for _, s := range sizes {
		if s == 3 {
			found3 = true
		}
		if s == 1 {
			found1 = true
		}
	}
	if !found3 || !found1 {
		t.Fatalf("expected group sizes [3,1], got %v", sizes)
	}
}
