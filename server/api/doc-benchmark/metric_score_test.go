package docbenchmark

import (
	"reflect"
	"testing"
)

func TestScoreMetricsDetectionEmptyRules(t *testing.T) {
	cases := []struct {
		name       string
		gold, pred []MetricRecord
		p, r, f    float64
	}{
		{"both", nil, nil, 1, 1, 1}, {"predictions", []MetricRecord{metric("g", 0, "n", "s", "1", "ms", 1)}, nil, 0, 0, 0},
		{"gold", nil, []MetricRecord{metric("", 7, "n", "s", "1", "ms", 1)}, 0, 1, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := ScoreMetrics(MetricScoreInput{Gold: tc.gold, Predictions: tc.pred})
			if value(s.DetectionPrecision) != tc.p || value(s.DetectionRecall) != tc.r || value(s.DetectionF1) != tc.f {
				t.Fatalf("detection = %#v %#v %#v", s.DetectionPrecision, s.DetectionRecall, s.DetectionF1)
			}
			if value(s.DuplicateRate) != 0 || value(s.UnsupportedRate) != boolFloat(len(tc.pred) > 0) {
				t.Fatalf("classification rates = %#v %#v", s.DuplicateRate, s.UnsupportedRate)
			}
		})
	}
}

func TestScoreMetricsMatchedAccuraciesGroundingAndDiagnostics(t *testing.T) {
	empty := ""
	g := metric("gold-b", 0, "Latency", "API", "100", "ms", 1, 2)
	g.NameEn = &empty
	g.Desc = ptr("response time")
	g.IsExplicitMetric = ptr(true)
	p := metric("", 12, "latency", "api", "1e2", "msec", 2, 3)
	p.NameEn = ptr("present")
	p.Desc = ptr("response time")
	p.IsExplicitMetric = ptr(false)
	s := ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: false, ArtifactHashes: map[string]string{"actual": "b", "gold": "a"}})
	if value(s.DetectionF1) != 1 || value(s.ValueAccuracy) != 1 || value(s.UnitAccuracy) != 1 || value(s.ValueUnitAccuracy) != 1 {
		t.Fatalf("matched scores = %#v", s)
	}
	if s.GroundingPrecision.TP != 1 || s.GroundingPrecision.FP != 1 || s.GroundingRecall.FN != 1 || value(s.GroundingF1) != 0.5 {
		t.Fatalf("grounding = %#v %#v %#v", s.GroundingPrecision, s.GroundingRecall, s.GroundingF1)
	}
	if value(s.StableFieldAccuracy["metric_desc"]) != 1 || value(s.StableFieldAccuracy["metric_name_en"]) != 0 {
		t.Fatalf("stable fields = %#v", s.StableFieldAccuracy)
	}
	if value(s.ExplicitAccuracy) != 0 {
		t.Fatalf("explicit = %#v", s.ExplicitAccuracy)
	}
	if len(s.Diagnostics.Accepted) != 1 || s.Diagnostics.Accepted[0].PredictionInputIndex != 12 || len(s.Diagnostics.FieldDifferences) == 0 {
		t.Fatalf("diagnostics = %#v", s.Diagnostics)
	}
	if !s.UpstreamInvalid || s.ConditionalAttributionIncluded {
		t.Fatalf("upstream flags = %#v", s)
	}
	if got := s.Diagnostics.ArtifactHashes; !reflect.DeepEqual(got, []ArtifactHash{{Name: "actual", Hash: "b"}, {Name: "gold", Hash: "a"}}) {
		t.Fatalf("hashes = %#v", got)
	}
	if s.ScorerVersion == "" || s.ScorerHash == "" || s.NormalizationHash == "" || len(s.Rows) == 0 {
		t.Fatalf("versions/rows absent: %#v", s)
	}
}

func TestScoreMetricsDuplicateUnsupportedAndMatchedNulls(t *testing.T) {
	g := metric("g", 0, "latency", "api", "100", "ms", 1)
	p1 := metric("", 4, "latency", "api", "100", "ms", 1)
	p2 := metric("", 5, "latency", "api", "100", "ms", 1)
	p3 := metric("", 6, "hallucination", "moon", "blue", "widgets", 999)
	s := ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p1, p2, p3}, UpstreamValid: true})
	if s.DetectionPrecision.TP != 1 || s.DetectionPrecision.FP != 2 || s.DetectionRecall.FN != 0 {
		t.Fatalf("detection counts = %#v", s.DetectionPrecision)
	}
	if s.DuplicateRate.Numerator != 1 || s.UnsupportedRate.Numerator != 1 || s.DuplicateRate.Denominator != 3 {
		t.Fatalf("classification = %#v %#v", s.DuplicateRate, s.UnsupportedRate)
	}
	if !reflect.DeepEqual(s.Diagnostics.DuplicatePredictionIndices, []int{5}) || !reflect.DeepEqual(s.Diagnostics.UnsupportedPredictionIndices, []int{6}) {
		t.Fatalf("classes = %#v", s.Diagnostics)
	}
	if s.StableFieldAccuracy["metric_desc"].Value != nil {
		t.Fatalf("unspecified stable field should be null: %#v", s.StableFieldAccuracy["metric_desc"])
	}
	if !s.ConditionalAttributionIncluded || s.UpstreamInvalid {
		t.Fatalf("upstream = %#v", s)
	}
}

func TestScoreMetricsWrongValuesUnitsAbsentEmptyAndEmptyGrounding(t *testing.T) {
	empty := ""
	g := metric("g", 0, "latency", "api", "100", "ms", 1)
	p := metric("", 1, "latency", "api", "200", "s", 1)
	s := ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: true})
	if value(s.DetectionF1) != 1 || value(s.ValueAccuracy) != 0 || value(s.UnitAccuracy) != 0 || value(s.ValueUnitAccuracy) != 0 {
		t.Fatalf("wrong matched fields not detected: %#v", s)
	}
	if s.ExplicitAccuracy.Value != nil || value(s.GroundingF1) != 1 {
		t.Fatalf("classification/grounding = %#v %#v", s.ExplicitAccuracy, s.GroundingF1)
	}

	g = metric("g", 0, "latency", "api", "ignored", "ms", 9)
	g.Value = &empty
	p = metric("", 2, "latency", "api", "ignored", "ms", 9)
	p.Value = nil
	s = ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: true})
	if value(s.ValueAccuracy) != 0 {
		t.Fatalf("absent and empty scored equal: %#v", s.ValueAccuracy)
	}

	g = metric("g", 0, "latency", "api", "1", "ms")
	p = metric("", 3, "latency", "api", "1", "ms")
	s = ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: true})
	if s.GroundingPrecision.Value != nil || s.GroundingRecall.Value != nil || s.GroundingF1.Value != nil {
		t.Fatalf("empty grounding should be null: %#v %#v %#v", s.GroundingPrecision, s.GroundingRecall, s.GroundingF1)
	}
}

func TestScoreMetricsIsDeterministic(t *testing.T) {
	in := MetricScoreInput{Gold: []MetricRecord{metric("z", 0, "n", "s", "1", "ms", 2), metric("a", 0, "x", "y", "2", "s", 3)}, Predictions: []MetricRecord{metric("", 8, "n", "s", "1", "ms", 2)}, UpstreamValid: true, ArtifactHashes: map[string]string{"z": "2", "a": "1"}}
	if first, second := ScoreMetrics(in), ScoreMetrics(in); !reflect.DeepEqual(first, second) {
		t.Fatalf("nondeterministic score:\n%#v\n%#v", first, second)
	}
}

func TestMetricVersionsConfigurationHashChangesWithBehavior(t *testing.T) {
	a := MetricScorerConfigurationV1()
	original := a.Hash()
	b := a
	b.Threshold++
	if original == b.Hash() {
		t.Fatal("threshold mutation did not change hash")
	}
	b = MetricScorerConfigurationV1()
	b.UnitAliases["fortnight"] = "s"
	if original == b.Hash() {
		t.Fatal("alias mutation did not change hash")
	}
	if original != MetricScorerConfigurationV1().Hash() {
		t.Fatal("configuration hash is unstable")
	}
	for _, mutate := range []func(*MetricScorerConfiguration){
		func(c *MetricScorerConfiguration) { c.Weights["name"]++ },
		func(c *MetricScorerConfiguration) { c.Eligibility += " changed" },
		func(c *MetricScorerConfiguration) { c.TieRule += " changed" },
	} {
		changed := MetricScorerConfigurationV1()
		mutate(&changed)
		if changed.Hash() == original {
			t.Fatal("behavior mutation did not change hash")
		}
	}
}

func boolFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
