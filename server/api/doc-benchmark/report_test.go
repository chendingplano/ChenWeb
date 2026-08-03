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

// TestBenchmarkReportCostYieldSerializationRoundTrip is a struct-serialization
// round-trip: it asserts cost/yield fields survive RenderJSON/Unmarshal. It is
// NOT the criterion-15 proof -- that is TestBenchmarkReportRoutingOffVsOnDiffers,
// which runs the analyzer over a routing-off/on pair (P5 review 2026080302
// criterion-15 note: the old test wore the criterion name while only round-
// tripping a hand-built report).
func TestBenchmarkReportCostYieldSerializationRoundTrip(t *testing.T) {
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

// TestBenchmarkReportRecallPrecisionSerializationRoundTrip is a
// struct-serialization round-trip for recall/precision aggregates. It is NOT
// the criterion-15 proof -- that is TestBenchmarkReportRoutingOffVsOnDiffers
// (P5 review 2026080302 criterion-15 note).
func TestBenchmarkReportRecallPrecisionSerializationRoundTrip(t *testing.T) {
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

// routingOffFixture is the routing-off (baseline) score set: every processor
// runs, so extract_metrics records processor_success and detection recall is
// full-yield; cost is the full-pipeline cost.
func routingOffFixture() []ScoreUnit {
	return []ScoreUnit{
		{CaseID: "case-1", Repetition: 1, Applicable: true, Tags: []string{"document_kind:product_specification"},
			Scores: []ScoreRow{
				{Metric: "processor_success", Component: "extract_metrics", AggregationKind: "binary_macro", Direction: "higher", Value: ptr(1.0), Numerator: 1, Denominator: 1},
				{Metric: "detection_recall", AggregationKind: "count_derived_micro", Direction: "higher", TP: 2, FP: 0, FN: 0},
				{Metric: "detection_precision", AggregationKind: "count_derived_micro", Direction: "higher", TP: 2, FP: 0, FN: 0},
				{Metric: "cost", AggregationKind: "operational", Direction: "lower", Value: ptr(0.20)},
			}},
		{CaseID: "case-2", Repetition: 1, Applicable: true,
			Scores: []ScoreRow{
				{Metric: "processor_success", Component: "extract_metrics", AggregationKind: "binary_macro", Direction: "higher", Value: ptr(1.0), Numerator: 1, Denominator: 1},
				{Metric: "detection_recall", AggregationKind: "count_derived_micro", Direction: "higher", TP: 1, FP: 0, FN: 0},
				{Metric: "detection_precision", AggregationKind: "count_derived_micro", Direction: "higher", TP: 1, FP: 0, FN: 0},
				{Metric: "cost", AggregationKind: "operational", Direction: "lower", Value: ptr(0.10)},
			}},
	}
}

// routingOnFixture is the routing-on (candidate) score set: extract_metrics
// was skipped by a gate rule with a documented reason, so its
// processor_success slice is absent, cost is lower, and detection recall drops
// because the skipped extraction is missing evidence. The skip reason is
// carried as a case tag so the report's slices attribute the skipped
// population (spec 2026080102 section 9: "every proposed skip/defer and its
// explanation trace").
func routingOnFixture() []ScoreUnit {
	return []ScoreUnit{
		{CaseID: "case-1", Repetition: 1, Applicable: true, Tags: []string{"document_kind:product_specification", "skip:extract_metrics:gate_rule_governed_doc_kind"},
			Scores: []ScoreRow{
				{Metric: "detection_recall", AggregationKind: "count_derived_micro", Direction: "higher", TP: 1, FP: 0, FN: 1},
				{Metric: "detection_precision", AggregationKind: "count_derived_micro", Direction: "higher", TP: 1, FP: 0, FN: 0},
				{Metric: "cost", AggregationKind: "operational", Direction: "lower", Value: ptr(0.05)},
			}},
		{CaseID: "case-2", Repetition: 1, Applicable: true, Tags: []string{"skip:extract_metrics:gate_rule_governed_doc_kind"},
			Scores: []ScoreRow{
				{Metric: "detection_recall", AggregationKind: "count_derived_micro", Direction: "higher", TP: 1, FP: 0, FN: 0},
				{Metric: "detection_precision", AggregationKind: "count_derived_micro", Direction: "higher", TP: 1, FP: 0, FN: 0},
				{Metric: "cost", AggregationKind: "operational", Direction: "lower", Value: ptr(0.02)},
			}},
	}
}

// TestBenchmarkReportRoutingOffVsOnDiffers is the criterion-15 proof (spec
// 2026080102 section 12 criterion 15): the analyzer run over a routing-off vs
// routing-on fixture pair records cost, yield (processor_success), and
// recall/precision that differ as expected, and attributes the explainable
// skip to its gate-rule reason. It replaces the two serialization round-trips
// that previously wore the criterion name (P5 review 2026080302 criterion-15
// note).
func TestBenchmarkReportRoutingOffVsOnDiffers(t *testing.T) {
	off, on := routingOffFixture(), routingOnFixture()

	aggOff, err := AggregateScores(off, len(off))
	if err != nil {
		t.Fatalf("AggregateScores(off): %v", err)
	}
	aggOn, err := AggregateScores(on, len(on))
	if err != nil {
		t.Fatalf("AggregateScores(on): %v", err)
	}

	rowOff := aggregateBy(aggOff, "processor_success", "extract_metrics")
	rowOn := aggregateBy(aggOn, "processor_success", "extract_metrics")
	if rowOff == nil || rowOn != nil {
		t.Fatalf("yield: extract_metrics processor_success present off=%v on=%v, want only off", rowOff != nil, rowOn != nil)
	}
	if rowOff.Value == nil || *rowOff.Value != 1.0 {
		t.Fatalf("processor_success off value = %v, want 1.0", rowOff.Value)
	}

	costOff := aggregateBy(aggOff, "cost", "")
	costOn := aggregateBy(aggOn, "cost", "")
	if costOff == nil || costOn == nil || costOff.Value == nil || costOn.Value == nil {
		t.Fatalf("cost aggregates missing: off=%v on=%v", costOff, costOn)
	}
	if *costOff.Value <= *costOn.Value {
		t.Fatalf("cost must be lower with routing on: off=%.3f on=%.3f", *costOff.Value, *costOn.Value)
	}

	recallOff := aggregateBy(aggOff, "detection_recall", "")
	recallOn := aggregateBy(aggOn, "detection_recall", "")
	if recallOff == nil || recallOn == nil || recallOff.Value == nil || recallOn.Value == nil {
		t.Fatalf("detection_recall aggregates missing: off=%v on=%v", recallOff, recallOn)
	}
	if *recallOff.Value <= *recallOn.Value {
		t.Fatalf("detection_recall must drop when the skipped processor's extraction is missing: off=%.3f on=%.3f", *recallOff.Value, *recallOn.Value)
	}

	// Paired deltas: routing-on is strictly worse on recall (negative delta).
	deltas, _, err := CompareVariants(VariantComparison{
		Baseline: off, Candidate: on,
		DatasetHash: "dataset-hash", BaselineCaseSetHash: "case-set-h", CandidateCaseSetHash: "case-set-h",
		ScorerVersion: "scorer-v", NormalizationVersion: "norm-v",
	})
	if err != nil {
		t.Fatalf("CompareVariants: %v", err)
	}
	delta := pairedDeltaBy(deltas, "detection_recall", "")
	if delta == nil || delta.Delta == nil || *delta.Delta >= 0 {
		t.Fatalf("detection_recall paired delta = %+v, want negative", delta)
	}

	// Explainable skip attribution: the routing-on slices carry the skip tag.
	slices, err := AggregateSlices(on, len(on))
	if err != nil {
		t.Fatalf("AggregateSlices(on): %v", err)
	}
	if _, ok := slices["skip:extract_metrics:gate_rule_governed_doc_kind"]; !ok {
		t.Fatalf("routing-on slices missing the explainable-skip tag, got %v", keys(slices))
	}
}

func aggregateBy(rows []AggregateRow, metric, component string) *AggregateRow {
	for i := range rows {
		if rows[i].Metric == metric && rows[i].Component == component {
			return &rows[i]
		}
	}
	return nil
}

func pairedDeltaBy(deltas []PairedDelta, metric, component string) *PairedDelta {
	for i := range deltas {
		if deltas[i].Metric == metric && deltas[i].Component == component {
			return &deltas[i]
		}
	}
	return nil
}

func keys(m map[string][]AggregateRow) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
