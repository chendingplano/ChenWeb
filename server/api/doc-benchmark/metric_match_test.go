package docbenchmark

import (
	"reflect"
	"testing"
)

func metric(id string, index int, name, subject, value, unit string, lines ...int) MetricRecord {
	return MetricRecord{GoldID: id, PredictionInputIndex: index, Name: ptr(name), Subject: ptr(subject), Value: ptr(value), Unit: ptr(unit), SourceLines: lines}
}

func TestMetricEdgesEligibilityWeightsAndThreshold(t *testing.T) {
	g := metric("g", 0, "request latency", "api", "100", "ms", 1, 2)
	p := metric("", 4, "request latency", "api", "1e2", "msec", 2, 3)
	e := MetricEdgeFor(g, p)
	if !e.Eligible || !e.Acceptable {
		t.Fatalf("edge = %#v", e)
	}
	if e.Components.Source != 116666 || e.Components.Name != 200000 || e.Components.Subject != 150000 || e.Components.Value != 200000 || e.Components.Unit != 100000 || e.Weight != 766666 {
		t.Fatalf("components = %#v weight=%d", e.Components, e.Weight)
	}

	// Two exact nonempty fields are eligible without a span intersection, but this
	// deliberately lands below the acceptance threshold.
	low := MetricEdgeFor(metric("g", 0, "same", "different", "1", "ms"), metric("", 0, "same", "other", "2", "ms"))
	if !low.Eligible || low.Acceptable || low.Weight != 300000 {
		t.Fatalf("low edge = %#v", low)
	}
	one := MetricEdgeFor(metric("g", 0, "same", "x", "1", "ms"), metric("", 0, "same", "y", "2", "s"))
	if one.Eligible {
		t.Fatalf("one exact field made eligible: %#v", one)
	}
	atThreshold := MetricEdgeFor(metric("g", 0, "left", "same", "1", "ms", 8), metric("", 0, "right", "same", "2", "ms", 8))
	if atThreshold.Weight != MetricAcceptanceWeight || !atThreshold.Acceptable {
		t.Fatalf("edge at exact threshold = %#v", atThreshold)
	}
}

func TestMatchMetricsFindsGlobalOptimumAndRectangularForbidden(t *testing.T) {
	edges := []MetricEdge{
		{GoldID: "a", GoldIndex: 0, PredictionInputIndex: 0, PredictionIndex: 0, Eligible: true, Acceptable: true, Weight: 800000},
		{GoldID: "a", GoldIndex: 0, PredictionInputIndex: 1, PredictionIndex: 1, Eligible: true, Acceptable: true, Weight: 700000},
		{GoldID: "b", GoldIndex: 1, PredictionInputIndex: 0, PredictionIndex: 0, Eligible: true, Acceptable: true, Weight: 700000},
	}
	got := optimalMetricMatches(2, 3, edges)
	want := []MetricMatch{{GoldID: "a", GoldIndex: 0, PredictionInputIndex: 1, PredictionIndex: 1, Weight: 700000}, {GoldID: "b", GoldIndex: 1, PredictionInputIndex: 0, PredictionIndex: 0, Weight: 700000}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("matches = %#v, want %#v", got, want)
	}
}

func TestMatchMetricsEqualOptimumUsesLexicographicallySmallestPairs(t *testing.T) {
	edges := []MetricEdge{
		{GoldID: "a", GoldIndex: 1, PredictionInputIndex: 9, PredictionIndex: 0, Acceptable: true, Weight: 600000},
		{GoldID: "a", GoldIndex: 1, PredictionInputIndex: 3, PredictionIndex: 1, Acceptable: true, Weight: 600000},
		{GoldID: "b", GoldIndex: 0, PredictionInputIndex: 9, PredictionIndex: 0, Acceptable: true, Weight: 600000},
		{GoldID: "b", GoldIndex: 0, PredictionInputIndex: 3, PredictionIndex: 1, Acceptable: true, Weight: 600000},
	}
	got := optimalMetricMatches(2, 2, edges)
	pairs := [][2]any{{got[0].GoldID, got[0].PredictionInputIndex}, {got[1].GoldID, got[1].PredictionInputIndex}}
	want := [][2]any{{"a", 3}, {"b", 9}}
	if !reflect.DeepEqual(pairs, want) {
		t.Fatalf("pairs = %#v, want %#v", pairs, want)
	}
}
