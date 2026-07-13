package docbenchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

const MetricScorerVersion = "metric-scorer-v1"

type MetricScoreInput struct {
	Gold, Predictions []MetricRecord
	UpstreamValid     bool
	ArtifactHashes    map[string]string
}

type ScoreRow struct {
	Metric                             string   `json:"metric"`
	Component                          string   `json:"component,omitempty"`
	Direction                          string   `json:"direction"`
	AggregationKind                    string   `json:"aggregation_kind"`
	Value                              *float64 `json:"value"`
	Numerator, Denominator, TP, FP, FN int
}

type AcceptedMetricDiagnostic struct {
	GoldID               string
	PredictionInputIndex int
	Weight               int
	Components           MetricEdgeComponents
}
type FieldDifference struct {
	GoldID, Field, Expected, Actual string
	PredictionInputIndex            int
}
type UnsupportedSourceSpan struct {
	PredictionInputIndex int
	SourceLines          []int
}
type MetricDiagnostics struct {
	Accepted                                                           []AcceptedMetricDiagnostic
	UnmatchedGoldIDs                                                   []string
	UnmatchedPredictionIndices                                         []int
	FieldDifferences                                                   []FieldDifference
	DuplicatePredictionIndices                                         []int
	UnsupportedPredictionIndices                                       []int
	UnsupportedSourceSpans                                             []UnsupportedSourceSpan
	ArtifactHashes                                                     []ArtifactHash
	ScorerVersion, ScorerHash, NormalizationVersion, NormalizationHash string
}

type MetricScore struct {
	DetectionPrecision, DetectionRecall, DetectionF1                   ScoreMetric
	ValueAccuracy, UnitAccuracy, ValueUnitAccuracy                     ScoreMetric
	GroundingPrecision, GroundingRecall, GroundingF1                   ScoreMetric
	StableFieldAccuracy                                                map[string]ScoreMetric
	ExplicitAccuracy                                                   ScoreMetric
	DuplicateRate, UnsupportedRate                                     ScoreMetric
	Rows                                                               []ScoreRow
	Diagnostics                                                        MetricDiagnostics
	ScorerVersion, ScorerHash, NormalizationVersion, NormalizationHash string
	UpstreamInvalid, ConditionalAttributionIncluded                    bool
}

type MetricScorerConfiguration struct {
	NormalizationVersion string            `json:"normalization_version"`
	UnitAliases          map[string]string `json:"unit_aliases"`
	Weights              map[string]int    `json:"weights_millionths"`
	Eligibility          string            `json:"eligibility"`
	Threshold            int               `json:"threshold_millionths"`
	TieRule              string            `json:"tie_rule"`
}

func MetricScorerConfigurationV1() MetricScorerConfiguration {
	aliases := make(map[string]string, len(unitAliasesV1))
	for k, v := range unitAliasesV1 {
		aliases[k] = v
	}
	return MetricScorerConfiguration{NormalizationVersion: NormalizationVersion, UnitAliases: aliases,
		Weights:     map[string]int{"source": 350000, "name": 200000, "subject": 150000, "value": 200000, "unit": 100000},
		Eligibility: "source_sets_intersect OR >=2 nonempty_present_exact(name,subject,value,unit)", Threshold: MetricAcceptanceWeight,
		TieRule: "lexicographically_smallest_ordered_(gold_id,prediction_input_index)_pairs"}
}
func (c MetricScorerConfiguration) Hash() string {
	raw, _ := json.Marshal(c)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func normalizationHashV1() string {
	raw, _ := json.Marshal(struct {
		Version string
		Aliases map[string]string
		Text    string
	}{NormalizationVersion, MetricScorerConfigurationV1().UnitAliases, "NFKC+Unicode-case-fold+whitespace-collapse+edge-punctuation; decimal-base10; sorted-dedup-lines"})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func ScoreMetrics(in MetricScoreInput) MetricScore {
	cfg := MetricScorerConfigurationV1()
	s := MetricScore{StableFieldAccuracy: make(map[string]ScoreMetric), ScorerVersion: MetricScorerVersion, ScorerHash: cfg.Hash(), NormalizationVersion: NormalizationVersion, NormalizationHash: normalizationHashV1(), UpstreamInvalid: !in.UpstreamValid, ConditionalAttributionIncluded: in.UpstreamValid}
	matches := MatchMetrics(in.Gold, in.Predictions)
	matchedGold, matchedPred := map[int]bool{}, map[int]bool{}
	for _, m := range matches {
		matchedGold[m.GoldIndex] = true
		matchedPred[m.PredictionIndex] = true
		s.Diagnostics.Accepted = append(s.Diagnostics.Accepted, AcceptedMetricDiagnostic{m.GoldID, m.PredictionInputIndex, m.Weight, m.Components})
	}
	tp, fp, fn := len(matches), len(in.Predictions)-len(matches), len(in.Gold)-len(matches)
	s.DetectionPrecision, s.DetectionRecall, s.DetectionF1 = detectionScores(tp, fp, fn)

	stable := stableFieldAccessors()
	for _, f := range stable {
		s.StableFieldAccuracy[f.name] = nullMetric()
	}
	fieldCorrect, fieldTotal := map[string]int{}, map[string]int{}
	valueCorrect, valueTotal, unitCorrect, unitTotal, bothCorrect, bothTotal := 0, 0, 0, 0, 0, 0
	explicitCorrect, explicitTotal := 0, 0
	groundTP, groundFP, groundFN := 0, 0, 0
	for _, m := range matches {
		g, p := in.Gold[m.GoldIndex], in.Predictions[m.PredictionIndex]
		gv, pv := NormalizeValue(g.Value), NormalizeValue(p.Value)
		gu, pu := NormalizeUnit(g.Unit), NormalizeUnit(p.Unit)
		valueOK, unitOK := normalizedFieldValueEqual(gv, pv), normalizedFieldUnitEqual(gu, pu)
		if g.Value != nil {
			valueTotal++
			if valueOK {
				valueCorrect++
			} else {
				s.addDiff(g, p, m, "metric_value", fieldDisplay(gv), fieldDisplay(pv))
			}
		}
		if g.Unit != nil {
			unitTotal++
			if unitOK {
				unitCorrect++
			} else {
				s.addDiff(g, p, m, "metric_unit", fieldDisplay(gu), fieldDisplay(pu))
			}
		}
		if g.Value != nil && g.Unit != nil {
			bothTotal++
			if valueOK && unitOK {
				bothCorrect++
			}
		}
		for _, f := range stable {
			gold := f.get(g)
			if gold == nil {
				continue
			}
			fieldTotal[f.name]++
			ng, np := NormalizeText(gold), NormalizeText(f.get(p))
			if fieldsEqual(ng, np) {
				fieldCorrect[f.name]++
			} else {
				s.addDiff(g, p, m, f.name, fieldDisplay(ng), fieldDisplay(np))
			}
		}
		if g.IsExplicitMetric != nil {
			explicitTotal++
			if p.IsExplicitMetric != nil && *g.IsExplicitMetric == *p.IsExplicitMetric {
				explicitCorrect++
			} else {
				s.addDiff(g, p, m, "is_explicit_metric", boolDisplay(g.IsExplicitMetric), boolDisplay(p.IsExplicitMetric))
			}
		}
		gl, pl := canonicalLines(g.SourceLines), canonicalLines(p.SourceLines)
		common := intIntersectionCount(gl, pl)
		groundTP += common
		groundFP += len(pl) - common
		groundFN += len(gl) - common
	}
	s.ValueAccuracy = nullableRatio(valueCorrect, valueTotal)
	s.UnitAccuracy = nullableRatio(unitCorrect, unitTotal)
	s.ValueUnitAccuracy = nullableRatio(bothCorrect, bothTotal)
	s.ExplicitAccuracy = nullableRatio(explicitCorrect, explicitTotal)
	for _, f := range stable {
		s.StableFieldAccuracy[f.name] = nullableRatio(fieldCorrect[f.name], fieldTotal[f.name])
	}
	if len(matches) > 0 {
		s.GroundingPrecision, s.GroundingRecall, s.GroundingF1 = groundingScores(groundTP, groundFP, groundFN)
	} else {
		s.GroundingPrecision, s.GroundingRecall, s.GroundingF1 = nullMetric(), nullMetric(), nullMetric()
	}

	edges := BuildMetricEdges(in.Gold, in.Predictions)
	for gi, g := range in.Gold {
		if !matchedGold[gi] {
			s.Diagnostics.UnmatchedGoldIDs = append(s.Diagnostics.UnmatchedGoldIDs, g.GoldID)
		}
	}
	duplicates, unsupported := 0, 0
	for pi, p := range in.Predictions {
		if matchedPred[pi] {
			continue
		}
		s.Diagnostics.UnmatchedPredictionIndices = append(s.Diagnostics.UnmatchedPredictionIndices, p.PredictionInputIndex)
		best := 0
		for _, e := range edges {
			if e.PredictionIndex == pi && e.Eligible && e.Weight > best {
				best = e.Weight
			}
		}
		if best >= MetricAcceptanceWeight {
			duplicates++
			s.Diagnostics.DuplicatePredictionIndices = append(s.Diagnostics.DuplicatePredictionIndices, p.PredictionInputIndex)
		} else {
			unsupported++
			s.Diagnostics.UnsupportedPredictionIndices = append(s.Diagnostics.UnsupportedPredictionIndices, p.PredictionInputIndex)
			s.Diagnostics.UnsupportedSourceSpans = append(s.Diagnostics.UnsupportedSourceSpans, UnsupportedSourceSpan{p.PredictionInputIndex, canonicalLines(p.SourceLines)})
		}
	}
	s.DuplicateRate = zeroRatio(duplicates, len(in.Predictions))
	s.UnsupportedRate = zeroRatio(unsupported, len(in.Predictions))
	s.Diagnostics.ArtifactHashes = sortedHashes(in.ArtifactHashes)
	s.Diagnostics.ScorerVersion = s.ScorerVersion
	s.Diagnostics.ScorerHash = s.ScorerHash
	s.Diagnostics.NormalizationVersion = s.NormalizationVersion
	s.Diagnostics.NormalizationHash = s.NormalizationHash
	sortMetricDiagnostics(&s.Diagnostics)
	s.Rows = metricScoreRows(s)
	return s
}

type fieldAccessor struct {
	name string
	get  func(MetricRecord) *string
}

func stableFieldAccessors() []fieldAccessor {
	return []fieldAccessor{
		{"metric_name", func(r MetricRecord) *string { return r.Name }}, {"metric_name_en", func(r MetricRecord) *string { return r.NameEn }}, {"metric_subject", func(r MetricRecord) *string { return r.Subject }}, {"metric_subject_en", func(r MetricRecord) *string { return r.SubjectEn }},
		{"metric_desc", func(r MetricRecord) *string { return r.Desc }}, {"metric_desc_en", func(r MetricRecord) *string { return r.DescEn }}, {"metric_context", func(r MetricRecord) *string { return r.Context }}, {"metric_context_en", func(r MetricRecord) *string { return r.ContextEn }},
		{"location_type", func(r MetricRecord) *string { return r.LocationType }}, {"value_data_type", func(r MetricRecord) *string { return r.ValueDataType }}, {"value_range_type", func(r MetricRecord) *string { return r.ValueRangeType }}, {"value_class", func(r MetricRecord) *string { return r.ValueClass }}, {"value_class_en", func(r MetricRecord) *string { return r.ValueClassEn }},
		{"formula_or_definition", func(r MetricRecord) *string { return r.FormulaOrDefinition }}, {"threshold_or_target", func(r MetricRecord) *string { return r.ThresholdOrTarget }}, {"measurement_frequency", func(r MetricRecord) *string { return r.MeasurementFrequency }}, {"table_name_or_section", func(r MetricRecord) *string { return r.TableNameOrSection }},
	}
}
func (s *MetricScore) addDiff(g, p MetricRecord, m MetricMatch, field, expected, actual string) {
	s.Diagnostics.FieldDifferences = append(s.Diagnostics.FieldDifferences, FieldDifference{GoldID: g.GoldID, Field: field, Expected: expected, Actual: actual, PredictionInputIndex: m.PredictionInputIndex})
}
func fieldDisplay(f NormalizedField) string {
	if f.State != FieldValue {
		return string(f.State)
	}
	return f.Text
}
func boolDisplay(v *bool) string {
	if v == nil {
		return "absent"
	}
	if *v {
		return "true"
	}
	return "false"
}
func normalizedFieldValueEqual(a, b NormalizedField) bool {
	if a.State != b.State {
		return false
	}
	if a.State != FieldValue {
		return true
	}
	return ValuesAgree(a, b)
}
func normalizedFieldUnitEqual(a, b NormalizedField) bool {
	if a.State != b.State {
		return false
	}
	if a.State != FieldValue {
		return true
	}
	return a.Text == b.Text
}
func nullMetric() ScoreMetric { return ScoreMetric{Value: nil} }
func nullableRatio(n, d int) ScoreMetric {
	if d == 0 {
		return nullMetric()
	}
	return ratioMetric(n, d)
}
func zeroRatio(n, d int) ScoreMetric {
	if d == 0 {
		return ratioMetric(0, 0)
	}
	return ratioMetric(n, d)
}
func detectionScores(tp, fp, fn int) (ScoreMetric, ScoreMetric, ScoreMetric) {
	if tp == 0 && fp == 0 && fn == 0 {
		p, r, f := ratioMetric(1, 1), ratioMetric(1, 1), ratioMetric(1, 1)
		p.TP = tp
		p.FP = fp
		p.FN = fn
		r.TP = tp
		r.FP = fp
		r.FN = fn
		f.TP = tp
		f.FP = fp
		f.FN = fn
		return p, r, f
	}
	if tp == 0 && fp == 0 {
		p, r, f := ratioMetric(0, 1), ratioMetric(0, 1), ratioMetric(0, 1)
		setCounts(&p, tp, fp, fn)
		setCounts(&r, tp, fp, fn)
		setCounts(&f, tp, fp, fn)
		return p, r, f
	}
	if tp == 0 && fn == 0 {
		p, r, f := ratioMetric(0, 1), ratioMetric(1, 1), ratioMetric(0, 1)
		setCounts(&p, tp, fp, fn)
		setCounts(&r, tp, fp, fn)
		setCounts(&f, tp, fp, fn)
		return p, r, f
	}
	p := ratioMetric(tp, tp+fp)
	r := ratioMetric(tp, tp+fn)
	f := ratioMetric(2*tp, 2*tp+fp+fn)
	setCounts(&p, tp, fp, fn)
	setCounts(&r, tp, fp, fn)
	setCounts(&f, tp, fp, fn)
	return p, r, f
}
func groundingScores(tp, fp, fn int) (ScoreMetric, ScoreMetric, ScoreMetric) {
	p := nullableRatio(tp, tp+fp)
	r := nullableRatio(tp, tp+fn)
	var f ScoreMetric
	if p.Value == nil || r.Value == nil || *p.Value+*r.Value == 0 {
		f = nullMetric()
	} else {
		v := 2 * *p.Value * *r.Value / (*p.Value + *r.Value)
		f = ScoreMetric{Value: &v, Numerator: 2 * tp, Denominator: 2*tp + fp + fn}
	}
	setCounts(&p, tp, fp, fn)
	setCounts(&r, tp, fp, fn)
	setCounts(&f, tp, fp, fn)
	return p, r, f
}
func setCounts(m *ScoreMetric, tp, fp, fn int) { m.TP = tp; m.FP = fp; m.FN = fn }
func sortMetricDiagnostics(d *MetricDiagnostics) {
	sort.Strings(d.UnmatchedGoldIDs)
	sort.Ints(d.UnmatchedPredictionIndices)
	sort.Ints(d.DuplicatePredictionIndices)
	sort.Ints(d.UnsupportedPredictionIndices)
	sort.Slice(d.FieldDifferences, func(i, j int) bool {
		a, b := d.FieldDifferences[i], d.FieldDifferences[j]
		if a.GoldID != b.GoldID {
			return a.GoldID < b.GoldID
		}
		if a.PredictionInputIndex != b.PredictionInputIndex {
			return a.PredictionInputIndex < b.PredictionInputIndex
		}
		return a.Field < b.Field
	})
}
func metricScoreRows(s MetricScore) []ScoreRow {
	rows := []ScoreRow{}
	add := func(name, dir, agg string, m ScoreMetric) {
		rows = append(rows, ScoreRow{Metric: name, Direction: dir, AggregationKind: agg, Value: m.Value, Numerator: m.Numerator, Denominator: m.Denominator, TP: m.TP, FP: m.FP, FN: m.FN})
	}
	add("detection_precision", "higher", "count_derived_micro", s.DetectionPrecision)
	add("detection_recall", "higher", "count_derived_micro", s.DetectionRecall)
	add("detection_f1", "higher", "count_derived_micro", s.DetectionF1)
	add("value_accuracy", "higher", "matched_field_micro", s.ValueAccuracy)
	add("unit_accuracy", "higher", "matched_field_micro", s.UnitAccuracy)
	add("value_unit_accuracy", "higher", "matched_field_micro", s.ValueUnitAccuracy)
	add("grounding_precision", "higher", "count_derived_micro", s.GroundingPrecision)
	add("grounding_recall", "higher", "count_derived_micro", s.GroundingRecall)
	add("grounding_f1", "higher", "count_derived_micro", s.GroundingF1)
	add("duplicate_prediction_rate", "lower", "binary_rate_macro", s.DuplicateRate)
	add("unsupported_metric_rate", "lower", "binary_rate_macro", s.UnsupportedRate)
	add("explicit_implicit_accuracy", "higher", "matched_field_micro", s.ExplicitAccuracy)
	addComponents := func(metric string, m ScoreMetric) {
		for _, c := range []struct {
			name  string
			value int
		}{{"fn", m.FN}, {"fp", m.FP}, {"tp", m.TP}} {
			v := float64(c.value)
			rows = append(rows, ScoreRow{Metric: metric, Component: c.name, Direction: "additive", AggregationKind: "additive_component", Value: &v, Numerator: c.value, Denominator: 1})
		}
	}
	addComponents("detection", s.DetectionF1)
	addComponents("grounding", s.GroundingF1)
	names := make([]string, 0, len(s.StableFieldAccuracy))
	for n := range s.StableFieldAccuracy {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		add("stable_field_"+n, "higher", "matched_field_micro", s.StableFieldAccuracy[n])
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Metric != rows[j].Metric {
			return rows[i].Metric < rows[j].Metric
		}
		return rows[i].Component < rows[j].Component
	})
	return rows
}
