package docbenchmark

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"
)

const MetricScorerVersion = "metric-scorer-v1"
const ExpectedMetricScorerHashV1 = "99eb05b47d956a5c786a67c0158c8f81c332eec18327445b191de28f27285c33"
const ExpectedMetricNormalizationHashV1 = "2f7db6b1fe1c8c9b57432f68a0a47dd010fd394e7ef6b41eb86337834156c35b"

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
	ExactWeight          string
	Components           MetricEdgeComponents
	ExactComponents      MetricEdgeExactComponents
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
	ScorerVersion        string                  `json:"scorer_version"`
	NormalizationVersion string                  `json:"normalization_version"`
	NormalizationRules   map[string]string       `json:"normalization_rules"`
	UnitAliases          map[string]string       `json:"unit_aliases"`
	Weights              map[string]string       `json:"weights_rational"`
	Eligibility          string                  `json:"eligibility"`
	Threshold            string                  `json:"threshold_rational"`
	TieRule              string                  `json:"tie_rule"`
	ScoreRows            []MetricScoreDefinition `json:"score_rows"`
}

type MetricScoreDefinition struct {
	Name, Direction, AggregationKind, NullableSemantics string
	AdditiveComponents                                  []string `json:",omitempty"`
}

var metricScoreRegistryV1 = []MetricScoreDefinition{
	{"detection_precision", "higher", "count_derived_micro", "section_10_3_detection_empty_rules", nil},
	{"detection_recall", "higher", "count_derived_micro", "section_10_3_detection_empty_rules", nil},
	{"detection_f1", "higher", "count_derived_micro", "section_10_3_detection_empty_rules", nil},
	{"value_accuracy", "higher", "matched_field_micro", "null_when_denominator_zero", nil}, {"unit_accuracy", "higher", "matched_field_micro", "null_when_denominator_zero", nil}, {"value_unit_accuracy", "higher", "matched_field_micro", "null_when_denominator_zero", nil},
	{"grounding_precision", "higher", "count_derived_micro", "null_when_no_accepted_match_or_denominator_zero", nil}, {"grounding_recall", "higher", "count_derived_micro", "null_when_no_accepted_match_or_denominator_zero", nil}, {"grounding_f1", "higher", "count_derived_micro", "zero_for_disjoint_nonempty;null_when_undefined", nil},
	{"duplicate_prediction_rate", "lower", "binary_rate_macro", "zero_when_no_predictions", nil}, {"unsupported_metric_rate", "lower", "binary_rate_macro", "zero_when_no_predictions", nil}, {"explicit_implicit_accuracy", "higher", "matched_field_micro", "null_when_denominator_zero", nil},
	{"stable_field_*", "higher", "matched_field_micro", "only_gold_map_membership_or_explicit_legacy_presence;null_when_denominator_zero", nil},
	{"detection", "additive", "additive_component", "integer_count", []string{"tp", "fp", "fn"}}, {"grounding", "additive", "additive_component", "integer_count", []string{"tp", "fp", "fn"}},
}

func MetricScorerConfigurationV1() MetricScorerConfiguration {
	aliases := make(map[string]string, len(unitAliasesV1))
	for k, v := range unitAliasesV1 {
		aliases[k] = v
	}
	return MetricScorerConfiguration{ScorerVersion: MetricScorerVersion, NormalizationVersion: NormalizationVersion, NormalizationRules: normalizationRulesV1(), UnitAliases: aliases,
		Weights:     map[string]string{"source": "7/20", "name": "1/5", "subject": "3/20", "value": "1/5", "unit": "1/10"},
		Eligibility: "source_sets_intersect OR >=2 nonempty_present_exact(name,subject,value,unit)", Threshold: "3/5",
		TieRule: "maximum_exact_rational_total_then_lexicographically_smallest_ordered_(gold_id,prediction_input_index)_pairs", ScoreRows: append([]MetricScoreDefinition(nil), metricScoreRegistryV1...)}
}
func normalizationRulesV1() map[string]string {
	return map[string]string{"text": "NFKC+Unicode-case-fold+whitespace-collapse+edge-punctuation", "value": "exact-base10-decimal-then-normalized-text", "source_lines": "sorted-deduplicated", "stable_json": "map-membership-presence; null=absent; strings=text-v1; numbers=exact-rational; bool=exact; arrays=ordered-recursive; objects=sorted-key-recursive; metric_value=value-v1; metric_unit=unit-v1"}
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
		Rules   map[string]string
	}{NormalizationVersion, MetricScorerConfigurationV1().UnitAliases, normalizationRulesV1()})
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
		s.Diagnostics.Accepted = append(s.Diagnostics.Accepted, AcceptedMetricDiagnostic{m.GoldID, m.PredictionInputIndex, m.Weight, m.ExactWeight, m.Components, m.ExactComponents})
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
		gv, gValuePresent := recordValueField(g, "metric_value", g.Value, NormalizeValue)
		pv, _ := recordValueField(p, "metric_value", p.Value, NormalizeValue)
		gu, gUnitPresent := recordValueField(g, "metric_unit", g.Unit, NormalizeUnit)
		pu, _ := recordValueField(p, "metric_unit", p.Unit, NormalizeUnit)
		valueOK, unitOK := normalizedFieldValueEqual(gv, pv), normalizedFieldUnitEqual(gu, pu)
		if gValuePresent {
			valueTotal++
			if valueOK {
				valueCorrect++
			} else {
				s.addDiff(g, p, m, "metric_value", fieldDisplay(gv), fieldDisplay(pv))
			}
		}
		if gUnitPresent {
			unitTotal++
			if unitOK {
				unitCorrect++
			} else {
				s.addDiff(g, p, m, "metric_unit", fieldDisplay(gu), fieldDisplay(pu))
			}
		}
		if gValuePresent && gUnitPresent {
			bothTotal++
			if valueOK && unitOK {
				bothCorrect++
			}
		}
		goldFields, predFields := stableFieldValues(g), stableFieldValues(p)
		names := make([]string, 0, len(goldFields))
		for name := range goldFields {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fieldTotal[name]++
			ng, np := normalizeStableJSON(name, goldFields[name]), normalizeStableJSON(name, predFields[name])
			if ng == np {
				fieldCorrect[name]++
			} else {
				s.addDiff(g, p, m, name, ng, np)
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
	allNames := map[string]bool{}
	for _, f := range stable {
		allNames[f.name] = true
	}
	for name := range fieldTotal {
		allNames[name] = true
	}
	for name := range allNames {
		s.StableFieldAccuracy[name] = nullableRatio(fieldCorrect[name], fieldTotal[name])
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
		best := new(big.Rat)
		for _, e := range edges {
			if e.PredictionIndex == pi && e.Eligible && edgeRat(e).Cmp(best) > 0 {
				best = new(big.Rat).Set(edgeRat(e))
			}
		}
		if best.Cmp(metricThresholdRat()) >= 0 {
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

func recordValueField(r MetricRecord, name string, legacy *string, normalize func(*string) NormalizedField) (NormalizedField, bool) {
	if raw, ok := r.StableFields[name]; ok {
		if string(bytes.TrimSpace(raw)) == "null" {
			return NormalizedField{State: FieldAbsent}, true
		}
		var text string
		if json.Unmarshal(raw, &text) == nil {
			return normalize(&text), true
		}
		return NormalizedField{State: FieldValue, Text: normalizeCommon(string(raw))}, true
	}
	return normalize(legacy), legacy != nil
}

type fieldAccessor struct {
	name string
	get  func(MetricRecord) *string
}

func stableFieldAccessors() []fieldAccessor {
	return []fieldAccessor{
		{"metric_name", func(r MetricRecord) *string { return r.Name }}, {"metric_name_en", func(r MetricRecord) *string { return r.NameEn }}, {"metric_subject", func(r MetricRecord) *string { return r.Subject }}, {"metric_subject_en", func(r MetricRecord) *string { return r.SubjectEn }},
		{"metric_unit_en", func(r MetricRecord) *string { return r.UnitEn }},
		{"metric_desc", func(r MetricRecord) *string { return r.Desc }}, {"metric_desc_en", func(r MetricRecord) *string { return r.DescEn }}, {"metric_context", func(r MetricRecord) *string { return r.Context }}, {"metric_context_en", func(r MetricRecord) *string { return r.ContextEn }},
		{"location_type", func(r MetricRecord) *string { return r.LocationType }}, {"value_data_type", func(r MetricRecord) *string { return r.ValueDataType }}, {"value_range_type", func(r MetricRecord) *string { return r.ValueRangeType }}, {"value_class", func(r MetricRecord) *string { return r.ValueClass }}, {"value_class_en", func(r MetricRecord) *string { return r.ValueClassEn }},
		{"formula_or_definition", func(r MetricRecord) *string { return r.FormulaOrDefinition }}, {"threshold_or_target", func(r MetricRecord) *string { return r.ThresholdOrTarget }}, {"measurement_frequency", func(r MetricRecord) *string { return r.MeasurementFrequency }}, {"table_name_or_section", func(r MetricRecord) *string { return r.TableNameOrSection }},
	}
}
func stableFieldValues(r MetricRecord) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage)
	for _, f := range stableFieldAccessors() {
		if v := f.get(r); v != nil {
			raw, _ := json.Marshal(*v)
			out[f.name] = raw
		}
	}
	for name, value := range map[string]*string{"metric_value": r.Value, "metric_unit": r.Unit} {
		if value != nil {
			raw, _ := json.Marshal(*value)
			out[name] = raw
		}
	}
	if r.IsExplicitMetric != nil {
		raw, _ := json.Marshal(*r.IsExplicitMetric)
		out["is_explicit_metric"] = raw
	}
	for name, raw := range r.StableFields {
		out[name] = append(json.RawMessage(nil), raw...)
	}
	return out
}
func normalizeStableJSON(name string, raw json.RawMessage) string {
	if raw == nil {
		return "absent"
	}
	if string(bytes.TrimSpace(raw)) == "null" {
		return "absent"
	}
	if name == "metric_value" || name == "metric_unit" {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			var f NormalizedField
			if name == "metric_value" {
				f = NormalizeValue(&text)
			} else {
				f = NormalizeUnit(&text)
			}
			if f.Decimal != nil {
				return "decimal:" + f.Decimal.String()
			}
			return "string:" + string(f.State) + ":" + f.Text
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return "invalid:" + normalizeCommon(string(raw))
	}
	return canonicalStableValue(v)
}
func canonicalStableValue(v any) string {
	switch x := v.(type) {
	case nil:
		return "absent"
	case string:
		f := NormalizeText(&x)
		return "string:" + string(f.State) + ":" + f.Text
	case bool:
		if x {
			return "bool:true"
		}
		return "bool:false"
	case json.Number:
		if d, ok := new(big.Rat).SetString(string(x)); ok {
			return "number:" + canonicalRat(d)
		}
		return "number-invalid:" + string(x)
	case []any:
		parts := make([]string, len(x))
		for i, item := range x {
			parts[i] = canonicalStableValue(item)
		}
		return "array:[" + strings.Join(parts, ",") + "]"
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%q:%s", k, canonicalStableValue(x[k])))
		}
		return "object:{" + strings.Join(parts, ",") + "}"
	default:
		return fmt.Sprintf("unknown:%v", x)
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
	if p.Value == nil || r.Value == nil {
		f = nullMetric()
	} else {
		f = nullableRatio(2*tp, 2*tp+fp+fn)
	}
	setCounts(&p, tp, fp, fn)
	setCounts(&r, tp, fp, fn)
	setCounts(&f, tp, fp, fn)
	return p, r, f
}
func ValidateMetricScoreRows(rows []ScoreRow) error {
	defs := map[string]MetricScoreDefinition{}
	for _, d := range metricScoreRegistryV1 {
		defs[d.Name] = d
	}
	seen := map[string]map[string]bool{}
	for _, row := range rows {
		name := row.Metric
		if strings.HasPrefix(name, "stable_field_") {
			name = "stable_field_*"
		}
		d, ok := defs[name]
		if !ok {
			return fmt.Errorf("unregistered score row %q", row.Metric)
		}
		if row.Direction != d.Direction || row.AggregationKind != d.AggregationKind {
			return fmt.Errorf("score row %q metadata differs from registry", row.Metric)
		}
		if seen[name] == nil {
			seen[name] = map[string]bool{}
		}
		seen[name][row.Component] = true
	}
	for name, d := range defs {
		if name == "stable_field_*" {
			continue
		}
		components := seen[name]
		if len(components) == 0 {
			return fmt.Errorf("registry score row %q not emitted", name)
		}
		for _, component := range d.AdditiveComponents {
			if !components[component] {
				return fmt.Errorf("registry score row %q missing component %q", name, component)
			}
		}
	}
	return nil
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
	add := func(name string, m ScoreMetric) {
		d := metricScoreDefinition(name)
		rows = append(rows, ScoreRow{Metric: name, Direction: d.Direction, AggregationKind: d.AggregationKind, Value: m.Value, Numerator: m.Numerator, Denominator: m.Denominator, TP: m.TP, FP: m.FP, FN: m.FN})
	}
	add("detection_precision", s.DetectionPrecision)
	add("detection_recall", s.DetectionRecall)
	add("detection_f1", s.DetectionF1)
	add("value_accuracy", s.ValueAccuracy)
	add("unit_accuracy", s.UnitAccuracy)
	add("value_unit_accuracy", s.ValueUnitAccuracy)
	add("grounding_precision", s.GroundingPrecision)
	add("grounding_recall", s.GroundingRecall)
	add("grounding_f1", s.GroundingF1)
	add("duplicate_prediction_rate", s.DuplicateRate)
	add("unsupported_metric_rate", s.UnsupportedRate)
	add("explicit_implicit_accuracy", s.ExplicitAccuracy)
	addComponents := func(metric string, m ScoreMetric) {
		d := metricScoreDefinition(metric)
		for _, c := range []struct {
			name  string
			value int
		}{{"fn", m.FN}, {"fp", m.FP}, {"tp", m.TP}} {
			v := float64(c.value)
			rows = append(rows, ScoreRow{Metric: metric, Component: c.name, Direction: d.Direction, AggregationKind: d.AggregationKind, Value: &v, Numerator: c.value, Denominator: 1})
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
		add("stable_field_"+n, s.StableFieldAccuracy[n])
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Metric != rows[j].Metric {
			return rows[i].Metric < rows[j].Metric
		}
		return rows[i].Component < rows[j].Component
	})
	return rows
}
func metricScoreDefinition(name string) MetricScoreDefinition {
	lookup := name
	if strings.HasPrefix(name, "stable_field_") {
		lookup = "stable_field_*"
	}
	for _, d := range metricScoreRegistryV1 {
		if d.Name == lookup {
			return d
		}
	}
	panic("unregistered metric score row: " + name)
}
