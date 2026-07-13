package docbenchmark

import "testing"

func TestAggregateScoresDispatchesMicroAndMacro(t *testing.T) {

	u := []ScoreUnit{{CaseID: "a", Repetition: 1, Applicable: true, Scores: []ScoreRow{{Metric: "detection_precision", AggregationKind: "count_derived_micro", Direction: "higher", TP: 1, FP: 1, FN: 0}, {Metric: "exact_case_pass", AggregationKind: "binary_rate_macro", Direction: "higher", Value: ptr(1.0)}}}, {CaseID: "b", Repetition: 1, Applicable: true, Scores: []ScoreRow{{Metric: "detection_precision", AggregationKind: "count_derived_micro", Direction: "higher", TP: 0, FP: 1, FN: 1}, {Metric: "exact_case_pass", AggregationKind: "binary_rate_macro", Direction: "higher", Value: ptr(0.0)}}}}
	rows, err := AggregateScores(u, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Metric != "detection_precision" {
		t.Fatalf("rows=%+v", rows)
	}
	if rows[0].Value == nil || *rows[0].Value != 1.0/3.0 || rows[0].Numerator != 1 || rows[0].Denominator != 3 {
		t.Fatalf("micro=%+v", rows[0])
	}
	if rows[1].Value == nil || *rows[1].Value != 0.5 || rows[1].NonNullUnits != 2 {
		t.Fatalf("macro=%+v", rows[1])
	}
}

func TestAggregateSlicesUsesTagPopulationAndOperationalMap(t *testing.T) {
	units := []ScoreUnit{{CaseID: "a", Applicable: true, Tags: []string{"x"}, Operational: map[string]float64{"latency": 10}}, {CaseID: "b", Applicable: true, Tags: []string{"y"}, Operational: map[string]float64{"latency": 20}}}
	got, err := AggregateSlices(units, 99)
	if err != nil {
		t.Fatal(err)
	}
	if got["x"] == nil || got["x"][0].ApplicableTotal != 1 {
		t.Fatalf("slice population: %#v", got)
	}
}

func TestGroundingEmptyPoolIsNull(t *testing.T) {
	rows, err := AggregateScores([]ScoreUnit{{Applicable: true, Scores: []ScoreRow{{Metric: "grounding_precision", AggregationKind: "count_derived_micro", TP: 0, FP: 0, FN: 0}, {Metric: "grounding_recall", AggregationKind: "count_derived_micro", TP: 0, FP: 0, FN: 0}, {Metric: "grounding_f1", AggregationKind: "count_derived_micro", TP: 0, FP: 0, FN: 0}}}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if r.Value != nil {
			t.Fatalf("%s should be null", r.Metric)
		}
	}
}
