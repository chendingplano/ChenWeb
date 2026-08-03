package docbenchmark

import (
	"encoding/json"
	"strings"
	"testing"
)

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

func TestBenchmarkReportRecordsCostYield(t *testing.T) {
	cost := 0.042
	telemetryMean := 1.5
	r := BenchmarkReport{
		ID:            "cost-yield-1",
		EstimatedCost: &cost,
		Telemetry: map[string]AggregateRow{
			"tokens_per_doc": {Metric: "tokens_per_doc", Value: &telemetryMean, AggregationKind: "operational", Direction: "lower"},
		},
		PricingSnapshot: map[string]string{
			"input_per_1k":  "$0.003",
			"output_per_1k": "$0.006",
		},
		Pareto: []ParetoPoint{
			{Variant: "fast", Quality: ptr(0.80), Latency: ptr(120.0), Cost: ptr(0.01)},
			{Variant: "accurate", Quality: ptr(0.95), Latency: ptr(500.0), Cost: ptr(0.05)},
		},
	}

	raw, err := RenderJSON(r)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var decoded BenchmarkReport
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.EstimatedCost == nil || *decoded.EstimatedCost != cost {
		t.Fatalf("EstimatedCost = %v, want %v", decoded.EstimatedCost, cost)
	}
	if len(decoded.Telemetry) != 1 || decoded.Telemetry["tokens_per_doc"].Value == nil || *decoded.Telemetry["tokens_per_doc"].Value != telemetryMean {
		t.Fatalf("Telemetry = %+v, want tokens_per_doc=%v", decoded.Telemetry, telemetryMean)
	}
	if len(decoded.PricingSnapshot) != 2 || decoded.PricingSnapshot["input_per_1k"] != "$0.003" {
		t.Fatalf("PricingSnapshot = %+v", decoded.PricingSnapshot)
	}
	if len(decoded.Pareto) != 2 {
		t.Fatalf("Pareto length = %d, want 2", len(decoded.Pareto))
	}
	paretoByVariant := map[string]ParetoPoint{}
	for _, p := range decoded.Pareto {
		paretoByVariant[p.Variant] = p
	}
	if p, ok := paretoByVariant["fast"]; !ok || p.Cost == nil || *p.Cost != 0.01 {
		t.Fatalf("Pareto[fast] = %+v, want Cost=0.01", paretoByVariant["fast"])
	}
	if p, ok := paretoByVariant["accurate"]; !ok || p.Cost == nil || *p.Cost != 0.05 {
		t.Fatalf("Pareto[accurate] = %+v, want Cost=0.05", paretoByVariant["accurate"])
	}

	md := RenderMarkdown(r)
	if !strings.Contains(md, "Telemetry and cost") {
		t.Fatal("markdown missing 'Telemetry and cost' section")
	}
	if !strings.Contains(md, "estimated cost") {
		t.Fatal("markdown missing estimated cost line")
	}
	if !strings.Contains(md, "Pareto trade-offs") {
		t.Fatal("markdown missing 'Pareto trade-offs' section")
	}
	if !strings.Contains(md, "Pricing") {
		t.Fatal("markdown missing 'Pricing' section")
	}
}

func TestBenchmarkReportRecordsRecallPrecision(t *testing.T) {
	r := BenchmarkReport{
		ID: "recall-precision-1",
		Aggregates: []AggregateRow{
			{Metric: "detection_precision", Direction: "higher", AggregationKind: "count_derived_micro", Value: ptr(0.75), TP: 3, FP: 1, FN: 1, Numerator: 3, Denominator: 4, ApplicableTotal: 5},
			{Metric: "detection_recall", Direction: "higher", AggregationKind: "count_derived_micro", Value: ptr(0.75), TP: 3, FP: 1, FN: 1, Numerator: 3, Denominator: 4, ApplicableTotal: 5},
			{Metric: "grounding_precision", Direction: "higher", AggregationKind: "count_derived_micro", Value: ptr(0.5), TP: 2, FP: 2, FN: 0, Numerator: 2, Denominator: 4, ApplicableTotal: 5},
			{Metric: "grounding_recall", Direction: "higher", AggregationKind: "count_derived_micro", Value: ptr(1.0), TP: 2, FP: 0, FN: 0, Numerator: 2, Denominator: 2, ApplicableTotal: 5},
		},
	}

	raw, err := RenderJSON(r)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	var decoded BenchmarkReport
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(decoded.Aggregates) != 4 {
		t.Fatalf("got %d aggregates, want 4", len(decoded.Aggregates))
	}

	byMetric := map[string]AggregateRow{}
	for _, a := range decoded.Aggregates {
		byMetric[a.Metric] = a
	}
	for _, name := range []string{"detection_precision", "detection_recall", "grounding_precision", "grounding_recall"} {
		row, ok := byMetric[name]
		if !ok {
			t.Fatalf("missing aggregate %q", name)
		}
		if row.Value == nil {
			t.Fatalf("%s: value is nil", name)
		}
		if row.TP+row.FP+row.FN == 0 {
			t.Fatalf("%s: TP/FP/FN all zero", name)
		}
	}
	if dp := byMetric["detection_precision"]; *dp.Value != 0.75 || dp.TP != 3 || dp.FP != 1 {
		t.Fatalf("detection_precision = %+v", dp)
	}
	if gr := byMetric["grounding_recall"]; *gr.Value != 1.0 || gr.TP != 2 || gr.FN != 0 {
		t.Fatalf("grounding_recall = %+v", gr)
	}

	md := RenderMarkdown(r)
	if !strings.Contains(md, "detection_precision") {
		t.Fatal("markdown missing detection_precision")
	}
	if !strings.Contains(md, "detection_recall") {
		t.Fatal("markdown missing detection_recall")
	}
	if !strings.Contains(md, "grounding_precision") {
		t.Fatal("markdown missing grounding_precision")
	}
	if !strings.Contains(md, "grounding_recall") {
		t.Fatal("markdown missing grounding_recall")
	}
}
