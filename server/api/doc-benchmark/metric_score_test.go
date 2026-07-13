package docbenchmark

import (
	"encoding/json"
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

func TestScoreMetricsDisjointGroundingIsZeroNotNull(t *testing.T) {
	g := metric("g", 0, "latency", "api", "1", "ms", 1)
	p := metric("", 0, "latency", "api", "1", "ms", 2)
	s := ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: true})
	if s.GroundingPrecision.Value == nil || s.GroundingRecall.Value == nil || s.GroundingF1.Value == nil || value(s.GroundingPrecision) != 0 || value(s.GroundingRecall) != 0 || value(s.GroundingF1) != 0 {
		t.Fatalf("disjoint grounding = %#v %#v %#v", s.GroundingPrecision, s.GroundingRecall, s.GroundingF1)
	}
}

func TestScoreMetricsOneSidedGroundingF1IsZero(t *testing.T) {
	for _, tc := range []struct {
		name       string
		gold, pred []int
	}{{"gold-empty", nil, []int{2}}, {"prediction-empty", []int{1}, nil}} {
		t.Run(tc.name, func(t *testing.T) {
			g := metric("g", 0, "latency", "api", "1", "ms", tc.gold...)
			p := metric("", 0, "latency", "api", "1", "ms", tc.pred...)
			s := ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: true})
			if s.GroundingF1.Value == nil || value(s.GroundingF1) != 0 {
				t.Fatalf("F1 = %#v", s.GroundingF1)
			}
		})
	}
	s := ScoreMetrics(MetricScoreInput{UpstreamValid: true})
	if s.GroundingPrecision.Value != nil || s.GroundingRecall.Value != nil || s.GroundingF1.Value != nil {
		t.Fatalf("no matches grounding = %#v %#v %#v", s.GroundingPrecision, s.GroundingRecall, s.GroundingF1)
	}
}

func TestScoreMetricsStableFieldsOverrideCoreAndBoolean(t *testing.T) {
	core := func(name, subject, value, unit string, explicit any) map[string]json.RawMessage {
		m := map[string]json.RawMessage{"metric_name": json.RawMessage(`"` + name + `"`), "metric_subject": json.RawMessage(`"` + subject + `"`), "metric_value": json.RawMessage(`"` + value + `"`), "metric_unit": json.RawMessage(`"` + unit + `"`)}
		if explicit != nil {
			raw, _ := json.Marshal(explicit)
			m["is_explicit_metric"] = raw
		}
		return m
	}
	g := MetricRecord{GoldID: "g", StableFields: core("latency", "api", "1e2", "msec", true), SourceLines: []int{1}}
	p := MetricRecord{PredictionInputIndex: 4, StableFields: core("LATENCY", "API", "100", "ms", true), SourceLines: []int{1}}
	s := ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: true})
	if value(s.DetectionF1) != 1 || value(s.ValueAccuracy) != 1 || value(s.UnitAccuracy) != 1 || s.ExplicitAccuracy.Denominator != 1 || value(s.ExplicitAccuracy) != 1 {
		t.Fatalf("map-only core score = %#v", s)
	}
	// Explicit map values override contradictory legacy pointers, including empty/null.
	g.Name = ptr("wrong")
	p.Name = ptr("wrong-other")
	g.IsExplicitMetric = ptr(false)
	p.IsExplicitMetric = ptr(false)
	s = ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: true})
	if value(s.DetectionF1) != 1 || value(s.ExplicitAccuracy) != 1 {
		t.Fatalf("map did not override legacy: %#v", s)
	}
	g.StableFields["is_explicit_metric"] = json.RawMessage(`null`)
	s = ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: true})
	if s.ExplicitAccuracy.Value != nil {
		t.Fatalf("null boolean label entered classification denominator: %#v", s.ExplicitAccuracy)
	}
}

func TestScoreMetricsGenericStableFieldPresenceAndCanonicalJSON(t *testing.T) {
	g := metric("g", 0, "latency", "api", "1", "ms", 1)
	g.StableFields = map[string]json.RawMessage{"metric_unit_en": json.RawMessage(`" milliseconds "`), "metric_keywords": json.RawMessage(`["ＦＯＯ", "bar"]`), "confidence": json.RawMessage(`0.90`), "objects": json.RawMessage(`{"b":" B ","a":1}`), "explicit_null": json.RawMessage(`null`), "explicit_empty": json.RawMessage(`""`)}
	g.StableFields["reasoning_tags"] = json.RawMessage(`["A"]`)
	g.StableFields["metric_categories"] = json.RawMessage(`{"x":[1]}`)
	g.StableFields["category_paths"] = json.RawMessage(`[" Root "]`)
	p := metric("", 0, "latency", "api", "1", "ms", 1)
	p.StableFields = map[string]json.RawMessage{"metric_unit_en": json.RawMessage(`"MILLISECONDS"`), "metric_keywords": json.RawMessage(`["foo","BAR"]`), "confidence": json.RawMessage(`9e-1`), "objects": json.RawMessage(`{"a":1.0,"b":"b"}`), "explicit_null": json.RawMessage(`null`), "explicit_empty": json.RawMessage(`null`), "prediction_only": json.RawMessage(`true`)}
	p.StableFields["reasoning_tags"] = json.RawMessage(`["a"]`)
	p.StableFields["metric_categories"] = json.RawMessage(`{"x":[1.0]}`)
	p.StableFields["category_paths"] = json.RawMessage(`["root"]`)
	s := ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: true})
	for _, name := range []string{"metric_unit_en", "metric_keywords", "confidence", "objects", "explicit_null", "reasoning_tags", "metric_categories", "category_paths"} {
		if value(s.StableFieldAccuracy[name]) != 1 {
			t.Errorf("%s = %#v", name, s.StableFieldAccuracy[name])
		}
	}
	if value(s.StableFieldAccuracy["explicit_empty"]) != 0 {
		t.Fatalf("empty vs absent = %#v", s.StableFieldAccuracy["explicit_empty"])
	}
	if _, ok := s.StableFieldAccuracy["prediction_only"]; ok {
		t.Fatal("prediction-only field entered denominator")
	}
	if s.StableFieldAccuracy["omitted"].Value != nil {
		t.Fatal("omitted field should not have denominator")
	}
}

func TestScoreMetricsDedicatedValueUsesExplicitPresenceNotPointerNil(t *testing.T) {
	g := metric("g", 0, "latency", "api", "placeholder", "ms", 1)
	g.Value = nil
	g.StableFields = map[string]json.RawMessage{"metric_value": json.RawMessage(`null`)}
	p := metric("", 0, "latency", "api", "placeholder", "ms", 1)
	p.Value = nil
	s := ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: true})
	if s.ValueAccuracy.Denominator != 1 || value(s.ValueAccuracy) != 1 {
		t.Fatalf("explicit null value presence = %#v", s.ValueAccuracy)
	}
	g.StableFields["metric_value"] = json.RawMessage(`""`)
	s = ScoreMetrics(MetricScoreInput{Gold: []MetricRecord{g}, Predictions: []MetricRecord{p}, UpstreamValid: true})
	if s.ValueAccuracy.Denominator != 1 || value(s.ValueAccuracy) != 0 {
		t.Fatalf("explicit empty value = %#v", s.ValueAccuracy)
	}
}

func TestMetricGoldenVersionHashesAndRegistry(t *testing.T) {
	if got := MetricScorerConfigurationV1().Hash(); got != ExpectedMetricScorerHashV1 {
		t.Fatalf("scorer hash = %s, update intentionally from %s", got, ExpectedMetricScorerHashV1)
	}
	if got := normalizationHashV1(); got != ExpectedMetricNormalizationHashV1 {
		t.Fatalf("normalization hash = %s, update intentionally from %s", got, ExpectedMetricNormalizationHashV1)
	}
	if err := ValidateMetricScoreRows(metricScoreRows(MetricScore{StableFieldAccuracy: map[string]ScoreMetric{}})); err != nil {
		t.Fatalf("row registry drift: %v", err)
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
	b.Threshold = "600001/1000000"
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
		func(c *MetricScorerConfiguration) { c.Weights["name"] = "200001/1000000" },
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

func TestMetricScorerConfigurationIsDeepCopied(t *testing.T) {
	original := MetricScorerConfigurationV1().Hash()
	c := MetricScorerConfigurationV1()
	c.UnitAliases["ms"] = "mutated"
	c.Weights["name"] = "mutated"
	c.NormalizationRules["text"] = "mutated"
	c.ScoreRows[0].AdditiveComponents = append(c.ScoreRows[0].AdditiveComponents, "mutated")
	c.ScoreRows[len(c.ScoreRows)-1].AdditiveComponents[0] = "mutated"
	if got := MetricScorerConfigurationV1().Hash(); got != original || got != ExpectedMetricScorerHashV1 {
		t.Fatalf("global config mutated: %s want %s", got, original)
	}
}

func boolFloat(v bool) float64 {
	if v {
		return 1
	}
	return 0
}
