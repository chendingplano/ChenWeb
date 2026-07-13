package docbenchmark

import "testing"

func TestRenderReportDeterministic(t *testing.T) {
	r := BenchmarkReport{ID: "r1", GeneratedAt: "2026-01-01T00:00:00Z", Aggregates: []AggregateRow{{Metric: "z", Value: ptr(1.0)}, {Metric: "a", Value: ptr(0.0)}}}
	a, err := RenderJSON(r)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RenderJSON(r)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatal("non deterministic")
	}
	if len(a) == 0 {
		t.Fatal("empty")
	}
}
