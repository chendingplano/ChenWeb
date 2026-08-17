package assertions

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestParseThresholdOrTargetLowerBound(t *testing.T) {
	p := parseThresholdOrTarget("shall be at least 250 cd/m2")
	if p.AssertionKind != "lower_bound_requirement" || p.Comparator != ">=" {
		t.Fatalf("unexpected parse: %+v", p)
	}
	if p.NumericValue == nil || *p.NumericValue != 250 {
		t.Fatalf("expected numeric value 250, got %+v", p.NumericValue)
	}
}

func TestParseThresholdOrTargetUpperBound(t *testing.T) {
	p := parseThresholdOrTarget("no more than 10%")
	if p.AssertionKind != "upper_bound_requirement" || p.Comparator != "<=" {
		t.Fatalf("unexpected parse: %+v", p)
	}
	if p.NumericValue == nil || *p.NumericValue != 10 {
		t.Fatalf("expected numeric value 10, got %+v", p.NumericValue)
	}
}

func TestParseThresholdOrTargetRange(t *testing.T) {
	p := parseThresholdOrTarget("10 to 20 degrees C")
	if p.AssertionKind != "interval_requirement" || p.ValueForm != "range" {
		t.Fatalf("unexpected parse: %+v", p)
	}
	if p.LowerValue == nil || *p.LowerValue != 10 || p.UpperValue == nil || *p.UpperValue != 20 {
		t.Fatalf("unexpected bounds: lower=%v upper=%v", p.LowerValue, p.UpperValue)
	}
}

func TestParseThresholdOrTargetBareNumberIsObservedValue(t *testing.T) {
	p := parseThresholdOrTarget("measured 150 cd/m2")
	if p.AssertionKind != "observed_value" || p.Comparator != "=" {
		t.Fatalf("unexpected parse: %+v", p)
	}
}

func TestParseThresholdOrTargetUnparsedNeverFabricatesAValue(t *testing.T) {
	p := parseThresholdOrTarget("see appendix B for details")
	if p.ValueForm != "unparsed" || p.NumericValue != nil || p.LowerValue != nil || p.UpperValue != nil {
		t.Fatalf("expected unparsed with no numeric fields, got %+v", p)
	}
}

func TestParseThresholdOrTargetEmptyIsUnparsed(t *testing.T) {
	p := parseThresholdOrTarget("")
	if p.ValueForm != "unparsed" {
		t.Fatalf("expected unparsed for empty input, got %+v", p)
	}
}

func TestParseThresholdOrTargetChineseLowerBound(t *testing.T) {
	p := parseThresholdOrTarget("不低于250 cd/m²")
	if p.AssertionKind != "lower_bound_requirement" || p.Comparator != ">=" {
		t.Fatalf("unexpected parse: %+v", p)
	}
	if p.NumericValue == nil || *p.NumericValue != 250 {
		t.Fatalf("expected numeric value 250, got %+v", p.NumericValue)
	}
}

func TestParseThresholdOrTargetChineseUpperBound(t *testing.T) {
	p := parseThresholdOrTarget("不超过120 ms")
	if p.AssertionKind != "upper_bound_requirement" || p.Comparator != "<=" {
		t.Fatalf("unexpected parse: %+v", p)
	}
	if p.NumericValue == nil || *p.NumericValue != 120 {
		t.Fatalf("expected numeric value 120, got %+v", p.NumericValue)
	}
}

func TestParseThresholdOrTargetChineseRange(t *testing.T) {
	p := parseThresholdOrTarget("500:1 至 2000:1")
	if p.AssertionKind != "interval_requirement" || p.ValueForm != "range" {
		t.Fatalf("unexpected parse: %+v", p)
	}
	if p.LowerValue == nil || *p.LowerValue != 500 || p.UpperValue == nil || *p.UpperValue != 2000 {
		t.Fatalf("unexpected bounds: lower=%v upper=%v", p.LowerValue, p.UpperValue)
	}
}

// TestParseThresholdOrTargetIgnoresLeadingDistanceNumber locks in the
// comparator-anchoring fix (P3 review finding 2b): the threshold value is
// the number after the matched comparator keyword, not the first number
// anywhere in the string -- a leading distance/condition clause must not be
// mistaken for the value.
func TestParseThresholdOrTargetIgnoresLeadingDistanceNumber(t *testing.T) {
	p := parseThresholdOrTarget("在 1 m 距离处不低于 250 cd/m²")
	if p.AssertionKind != "lower_bound_requirement" || p.Comparator != ">=" {
		t.Fatalf("unexpected parse: %+v", p)
	}
	if p.NumericValue == nil || *p.NumericValue != 250 {
		t.Fatalf("expected numeric value 250 (not the leading distance '1'), got %+v", p.NumericValue)
	}
}

// TestParseThresholdOrTargetIgnoresStandardNumberDash locks in the range-
// separator fix: an unspaced ASCII hyphen inside a standard/document
// identifier ("9706.1-2020") must not be mistaken for a range separator,
// and the comparator keyword after it must still resolve to the real value.
func TestParseThresholdOrTargetIgnoresStandardNumberDash(t *testing.T) {
	p := parseThresholdOrTarget("GB 9706.1-2020 规定不低于 250")
	if p.ValueForm == "range" {
		t.Fatalf("expected the standard-number dash not to be parsed as a range, got %+v", p)
	}
	if p.AssertionKind != "lower_bound_requirement" || p.Comparator != ">=" {
		t.Fatalf("unexpected parse: %+v", p)
	}
	if p.NumericValue == nil || *p.NumericValue != 250 {
		t.Fatalf("expected numeric value 250, got %+v", p.NumericValue)
	}
}

// TestParseThresholdOrTargetChineseUpperBoundExtendedVocab covers the
// 不应超过/不得超过 upper-bound forms added by the P3 review fix.
func TestParseThresholdOrTargetChineseUpperBoundExtendedVocab(t *testing.T) {
	for _, s := range []string{"不应超过 5 %", "不得超过 5 %"} {
		p := parseThresholdOrTarget(s)
		if p.AssertionKind != "upper_bound_requirement" || p.Comparator != "<=" {
			t.Fatalf("unexpected parse for %q: %+v", s, p)
		}
		if p.NumericValue == nil || *p.NumericValue != 5 {
			t.Fatalf("expected numeric value 5 for %q, got %+v", s, p.NumericValue)
		}
	}
}

// TestParseThresholdOrTargetChineseLowerBoundExtendedVocab covers the
// 不得低于 lower-bound form added by the P3 review fix.
func TestParseThresholdOrTargetChineseLowerBoundExtendedVocab(t *testing.T) {
	p := parseThresholdOrTarget("不得低于 250 cd/m²")
	if p.AssertionKind != "lower_bound_requirement" || p.Comparator != ">=" {
		t.Fatalf("unexpected parse: %+v", p)
	}
	if p.NumericValue == nil || *p.NumericValue != 250 {
		t.Fatalf("expected numeric value 250, got %+v", p.NumericValue)
	}
}

// --- structured-first path (design D1) ---

// nullStr / numStr / numF are helpers for building metricRow fixtures for
// resolveMetricValue.
func ns(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nf(v float64) sql.NullFloat64 {
	return sql.NullFloat64{Float64: v, Valid: true}
}

func metricRowFixture() metricRow {
	return metricRow{MetricID: "m1", MetricUnit: ns("cd/m²")}
}

// resolveMetricValueLegacy preserves the pre-DR1 call convention
// (resolveMetricValue(r)) for tests exercising resolveMetricValue's
// structural parsing, not the governed DB-backed lookup itself --
// canonicalization here still goes through CanonicalMetricValueRangeType,
// which is retained exactly for this purpose (see its doc comment).
// TestResolveMetricValueCanonicalizesProductionVocabularySurvey and
// TestResolveMetricValueLeavesAmbiguousVocabularyUnparsed exercise
// ValueRangeTypeMapper.Lookup directly instead (ADR 2026081401 §11).
func resolveMetricValueLegacy(r metricRow) parsedThreshold {
	return resolveMetricValue(r, CanonicalMetricValueRangeType(r.ValueRangeType.String))
}

func metricCandidatePayloadForRowLegacy(r metricRow) map[string]any {
	return metricCandidatePayloadForRow(r, CanonicalMetricValueRangeType(r.ValueRangeType.String), "approved")
}

func TestResolveMetricValueLowerBound(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("lower_bound")
	r.ValueClass = ns("requirement")
	r.MetricValue = ns("250")
	p := resolveMetricValueLegacy(r)
	if p.ValueForm != "single" || p.Comparator != ">=" || p.AssertionKind != "lower_bound_requirement" {
		t.Fatalf("unexpected resolve: %+v", p)
	}
	if p.NumericValue == nil || *p.NumericValue != 250 {
		t.Fatalf("expected numeric value 250, got %+v", p.NumericValue)
	}
}

func TestResolveMetricValueUpperBound(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("upper_bound")
	r.ValueClass = ns("requirement")
	r.MetricValue = ns("120")
	p := resolveMetricValueLegacy(r)
	if p.Comparator != "<=" || p.AssertionKind != "upper_bound_requirement" {
		t.Fatalf("unexpected resolve: %+v", p)
	}
	if p.NumericValue == nil || *p.NumericValue != 120 {
		t.Fatalf("expected numeric value 120, got %+v", p.NumericValue)
	}
}

func TestResolveMetricValueExact(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("exact")
	r.ValueClass = ns("requirement")
	r.MetricValue = ns("15")
	p := resolveMetricValueLegacy(r)
	if p.Comparator != "=" || p.AssertionKind != "exact_value" {
		t.Fatalf("unexpected resolve: %+v", p)
	}
	if p.NumericValue == nil || *p.NumericValue != 15 {
		t.Fatalf("expected numeric value 15, got %+v", p.NumericValue)
	}
}

// TestResolveMetricValueCanonicalizesExtractorRangeVariants locks in the
// extractor/normalizer contract for the variants emitted by production
// extract_metrics runs. These must be canonicalized before the structured
// resolver decides an assertion kind; a nonempty synonym must not suppress
// an otherwise valid threshold.
func TestResolveMetricValueCanonicalizesExtractorRangeVariants(t *testing.T) {
	tests := []struct {
		name       string
		rangeType  string
		value      string
		comparator string
		kind       string
		want       float64
	}{
		{name: "min", rangeType: "min", value: "≥30", comparator: ">=", kind: "lower_bound_requirement", want: 30},
		{name: "minimum", rangeType: "minimum", value: "30", comparator: ">=", kind: "lower_bound_requirement", want: 30},
		{name: "min threshold", rangeType: "min_threshold", value: "30", comparator: ">=", kind: "lower_bound_requirement", want: 30},
		{name: "max", rangeType: "max", value: "≤30", comparator: "<=", kind: "upper_bound_requirement", want: 30},
		{name: "maximum", rangeType: "maximum", value: "30", comparator: "<=", kind: "upper_bound_requirement", want: 30},
		{name: "exact count", rangeType: "exact_count", value: "4", comparator: "=", kind: "exact_value", want: 4},
		{name: "exact duration", rangeType: "exact_duration", value: "15", comparator: "=", kind: "exact_value", want: 15},
		{name: "exact ratio", rangeType: "exact_ratio", value: "3", comparator: "=", kind: "exact_value", want: 3},
		{name: "exact specification", rangeType: "exact_specification", value: "12", comparator: "=", kind: "exact_value", want: 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := metricRowFixture()
			r.ValueRangeType = ns(tt.rangeType)
			r.ValueClass = ns("requirement")
			r.MetricValue = ns(tt.value)

			p := resolveMetricValueLegacy(r)
			if p.Comparator != tt.comparator || p.AssertionKind != tt.kind {
				t.Fatalf("resolveMetricValue(%q) = %+v, want comparator=%q kind=%q", tt.rangeType, p, tt.comparator, tt.kind)
			}
			if p.NumericValue == nil || *p.NumericValue != tt.want {
				t.Fatalf("numeric value = %v, want %v", p.NumericValue, tt.want)
			}
		})
	}
}

// TestResolveMetricValueCanonicalizesProductionVocabularySurvey locks in the
// 2026-08-14 synonym-table expansion, driven by a full survey of production
// kb.metrics.value_range_type values (309 distinct strings across 7,036
// rows). Only unambiguous-direction synonyms were added; this covers a
// representative sample from each bucket, including symbol and Chinese
// forms, not every surveyed string.
func TestResolveMetricValueCanonicalizesProductionVocabularySurvey(t *testing.T) {
	tests := []struct {
		name       string
		rangeType  string
		value      string
		comparator string
		kind       string
		want       float64
	}{
		{name: "greater_than_or_equal", rangeType: "greater_than_or_equal", value: "30", comparator: ">=", kind: "lower_bound_requirement", want: 30},
		{name: "greater than or equal to", rangeType: "greater than or equal to", value: "30", comparator: ">=", kind: "lower_bound_requirement", want: 30},
		{name: "at least", rangeType: "at least", value: "30", comparator: ">=", kind: "lower_bound_requirement", want: 30},
		{name: "symbol >=", rangeType: ">=", value: "30", comparator: ">=", kind: "lower_bound_requirement", want: 30},
		{name: "symbol ≥", rangeType: "≥", value: "30", comparator: ">=", kind: "lower_bound_requirement", want: 30},
		{name: "Chinese 大于等于", rangeType: "大于等于", value: "30", comparator: ">=", kind: "lower_bound_requirement", want: 30},
		{name: "Chinese 下限", rangeType: "下限", value: "30", comparator: ">=", kind: "lower_bound_requirement", want: 30},
		{name: "threshold_min", rangeType: "threshold_min", value: "30", comparator: ">=", kind: "lower_bound_requirement", want: 30},

		{name: "less_than_or_equal", rangeType: "less_than_or_equal", value: "30", comparator: "<=", kind: "upper_bound_requirement", want: 30},
		{name: "less than or equal to", rangeType: "less than or equal to", value: "30", comparator: "<=", kind: "upper_bound_requirement", want: 30},
		{name: "symbol <=", rangeType: "<=", value: "30", comparator: "<=", kind: "upper_bound_requirement", want: 30},
		{name: "symbol ≤", rangeType: "≤", value: "30", comparator: "<=", kind: "upper_bound_requirement", want: 30},
		{name: "Chinese 不大于", rangeType: "不大于", value: "30", comparator: "<=", kind: "upper_bound_requirement", want: 30},
		{name: "Chinese 上限", rangeType: "上限", value: "30", comparator: "<=", kind: "upper_bound_requirement", want: 30},

		{name: "single", rangeType: "single", value: "500", comparator: "=", kind: "exact_value", want: 500},
		{name: "single_value", rangeType: "single_value", value: "3", comparator: "=", kind: "exact_value", want: 3},
		{name: "integer", rangeType: "integer", value: "24", comparator: "=", kind: "exact_value", want: 24},
		{name: "fixed", rangeType: "fixed", value: "100", comparator: "=", kind: "exact_value", want: 100},
		{name: "Chinese 固定值", rangeType: "固定值", value: "100", comparator: "=", kind: "exact_value", want: 100},
		{name: "Chinese 单值", rangeType: "单值", value: "3", comparator: "=", kind: "exact_value", want: 3},
		{name: "Chinese 点值", rangeType: "点值", value: "3", comparator: "=", kind: "exact_value", want: 3},

		{name: "min_max", rangeType: "min-max", value: "5.5~8.5", comparator: "between", kind: "interval_requirement"},
		{name: "Chinese 区间", rangeType: "区间", value: "5.5~8.5", comparator: "between", kind: "interval_requirement"},
		{name: "Chinese 范围", rangeType: "范围", value: "5.5~8.5", comparator: "between", kind: "interval_requirement"},
	}
	// ADR 2026081401 §11: this test now drives resolveMetricValue via the
	// governed DB-backed ValueRangeTypeMapper (sqlmock, seeded to mirror the
	// DR5 migration's 'approved' rows) instead of calling
	// CanonicalMetricValueRangeType directly -- confirms no behavior
	// regression during cutover.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"raw_value", "canonical_bucket", "status"})
	for _, tt := range tests {
		rows.AddRow(normalizeValueRangeTypeRaw(tt.rangeType), CanonicalMetricValueRangeType(tt.rangeType), "approved")
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT raw_value, canonical_bucket, status FROM kb.metric_value_range_type_map")).
		WillReturnRows(rows)

	mapper := ValueRangeTypeMapper{DB: db, cache: &valueRangeTypeMapCache{}}
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := metricRowFixture()
			r.ValueRangeType = ns(tt.rangeType)
			r.ValueClass = ns("requirement")
			r.MetricValue = ns(tt.value)

			bucket, status, err := mapper.Lookup(ctx, tt.rangeType, 416)
			if err != nil {
				t.Fatalf("Lookup(%q) error: %v", tt.rangeType, err)
			}
			if status != "approved" {
				t.Fatalf("Lookup(%q) status = %q, want approved", tt.rangeType, status)
			}

			p := resolveMetricValue(r, bucket)
			if p.Comparator != tt.comparator || p.AssertionKind != tt.kind {
				t.Fatalf("resolveMetricValue(%q) = %+v, want comparator=%q kind=%q", tt.rangeType, p, tt.comparator, tt.kind)
			}
			if tt.kind == "interval_requirement" {
				if p.LowerValue == nil || *p.LowerValue != 5.5 || p.UpperValue == nil || *p.UpperValue != 8.5 {
					t.Fatalf("unexpected bounds: lower=%v upper=%v", p.LowerValue, p.UpperValue)
				}
				return
			}
			if p.NumericValue == nil || *p.NumericValue != tt.want {
				t.Fatalf("numeric value = %v, want %v", p.NumericValue, tt.want)
			}
		})
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestResolveMetricValueLeavesAmbiguousVocabularyUnparsed locks in the
// conservative half of the 2026-08-14 synonym-table expansion: strings whose
// direction can't be inferred (threshold, target, tolerance, categorical)
// must keep deferring rather than guess. ADR 2026081401 §11: driven through
// ValueRangeTypeMapper (sqlmock, seeded with DR5's 'ambiguous' rows) instead
// of CanonicalMetricValueRangeType directly, and additionally asserts the
// governed lookup itself reports status "ambiguous" with no usable bucket.
func TestResolveMetricValueLeavesAmbiguousVocabularyUnparsed(t *testing.T) {
	ambiguous := []string{"threshold", "target", "tolerance", "categorical", "ordinal", "continuous", "binary", "ratio"}

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows := sqlmock.NewRows([]string{"raw_value", "canonical_bucket", "status"})
	for _, vrt := range ambiguous {
		rows.AddRow(vrt, nil, "ambiguous")
	}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT raw_value, canonical_bucket, status FROM kb.metric_value_range_type_map")).
		WillReturnRows(rows)
	for range ambiguous {
		mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.metric_value_range_type_map")).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	mapper := ValueRangeTypeMapper{DB: db, cache: &valueRangeTypeMapCache{}}
	ctx := context.Background()

	for _, vrt := range ambiguous {
		r := metricRowFixture()
		r.ValueRangeType = ns(vrt)
		r.ValueClass = ns("requirement")
		r.MetricValue = ns("100")

		bucket, status, err := mapper.Lookup(ctx, vrt, 416)
		if err != nil {
			t.Fatalf("Lookup(%q) error: %v", vrt, err)
		}
		if status != "ambiguous" {
			t.Fatalf("Lookup(%q) status = %q, want ambiguous", vrt, status)
		}
		if bucket != "" {
			t.Fatalf("Lookup(%q) bucket = %q, want empty -- ambiguous is never authoritative", vrt, bucket)
		}

		p := resolveMetricValue(r, bucket)
		if p.ValueForm != "unparsed" || p.AssertionKind != "unparsed" {
			t.Fatalf("expected %q to remain unparsed (direction-ambiguous), got %+v", vrt, p)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMetricCandidatePayloadCarriesMetricDefinitionTermID(t *testing.T) {
	r := metricRowFixture()
	r.MetricName = ns("Organic matter")
	r.MetricDefinitionTermID = ns("mea:organic_matter_mass_fraction")
	r.ValueRangeType = ns("min")
	r.ValueClass = ns("requirement")
	r.MetricValue = ns("≥30")

	payload := metricCandidatePayloadForRowLegacy(r)
	if got := payload["metric_definition_term_id"]; got != "mea:organic_matter_mass_fraction" {
		t.Fatalf("metric_definition_term_id = %#v, want resolved term id", got)
	}
}

// TestMetricCandidatePayloadCarriesEvidenceProvenanceFields locks in that
// kb.metrics' own event_id/model_name/prompt_name reach the candidate
// payload as extraction_run/model/prompt_version, so associate_semantics can
// later copy them onto kb.assertion_evidence (spec 2026072702 §8.3).
func TestMetricCandidatePayloadCarriesEvidenceProvenanceFields(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("min")
	r.ValueClass = ns("requirement")
	r.MetricValue = ns("≥30")
	r.EventID = ns("evt-abc123")
	r.ModelName = ns("deepseek-chat")
	r.PromptName = ns("prompt-extract-metrics-v3")

	payload := metricCandidatePayloadForRowLegacy(r)
	if got := payload["extraction_run"]; got != "evt-abc123" {
		t.Fatalf("extraction_run = %#v, want evt-abc123", got)
	}
	if got := payload["model"]; got != "deepseek-chat" {
		t.Fatalf("model = %#v, want deepseek-chat", got)
	}
	if got := payload["prompt_version"]; got != "prompt-extract-metrics-v3" {
		t.Fatalf("prompt_version = %#v, want prompt-extract-metrics-v3", got)
	}
}

// TestMetricCandidatePayloadOmitsEmptyEvidenceProvenanceFields matches the
// existing condition/subject_object_id convention: an absent source value
// must not appear in the payload as an empty string.
func TestMetricCandidatePayloadOmitsEmptyEvidenceProvenanceFields(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("min")
	r.ValueClass = ns("requirement")
	r.MetricValue = ns("≥30")

	payload := metricCandidatePayloadForRowLegacy(r)
	for _, key := range []string{"extraction_run", "model", "prompt_version"} {
		if _, ok := payload[key]; ok {
			t.Fatalf("payload[%q] = %#v, want key absent when source column is empty", key, payload[key])
		}
	}
}

func TestResolveMetricValueProductionRatioDoesNotFabricateScalar(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("exact_ratio")
	r.ValueClass = ns("ratio")
	r.MetricValue = ns("1:10")
	p := resolveMetricValueLegacy(r)
	if p.ValueForm != "unparsed" || p.NumericValue != nil {
		t.Fatalf("exact ratio 1:10 must not become a scalar assertion, got %+v", p)
	}
}

func TestResolveMetricValueProductionRangeParsesMetricValue(t *testing.T) {
	for _, raw := range []string{"5.5~8.5", "0℃～4℃"} {
		r := metricRowFixture()
		r.ValueRangeType = ns("range")
		r.MetricValue = ns(raw)
		p := resolveMetricValueLegacy(r)
		if p.ValueForm != "range" || p.AssertionKind != "interval_requirement" || p.LowerValue == nil || p.UpperValue == nil {
			t.Fatalf("range %q = %+v, want parsed interval", raw, p)
		}
	}
}

func TestResolveMetricValueProductionCompoundRangesRemainUnparsed(t *testing.T) {
	for _, raw := range []string{
		"白天6:00～22:00; 夜间22:00～次日6:00",
		"I级: <30; II级: 30~40",
		"I级: <10; II级: 10~20",
		"I级: <15; II级: 15~20",
	} {
		r := metricRowFixture()
		r.ValueRangeType = ns("range")
		r.MetricValue = ns(raw)
		p := resolveMetricValueLegacy(r)
		if p.ValueForm != "unparsed" || p.LowerValue != nil || p.UpperValue != nil {
			t.Fatalf("compound range %q must remain unparsed, got %+v", raw, p)
		}
	}
}

// TestResolveMetricValueRangeUsesValueMinMax locks in design D1: a range row's
// endpoints come from value_min/value_max, never from re-parsing
// threshold_or_target (spec: "range values use value_min and value_max without
// free-text parsing"). The fixture's metric_value carries range-looking text
// that would be parsed by parseThresholdOrTarget -- if the structured path
// leaks to the parser, the bounds would come out wrong.
func TestResolveMetricValueRangeUsesValueMinMax(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("range")
	r.ValueClass = ns("requirement")
	r.MetricValue = ns("500:1 至 2000:1")
	r.ValueMin = nf(500)
	r.ValueMax = nf(2000)
	p := resolveMetricValueLegacy(r)
	if p.ValueForm != "range" || p.Comparator != "between" || p.AssertionKind != "interval_requirement" {
		t.Fatalf("unexpected resolve: %+v", p)
	}
	if p.LowerValue == nil || *p.LowerValue != 500 || p.UpperValue == nil || *p.UpperValue != 2000 {
		t.Fatalf("expected bounds 500/2000 from value_min/value_max, got lower=%v upper=%v", p.LowerValue, p.UpperValue)
	}
}

func TestResolveMetricValueQualitativeHasNoNumericFields(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("qualitative")
	r.ValueClass = ns("observation")
	r.MetricValue = ns("clearly legible")
	p := resolveMetricValueLegacy(r)
	if p.ValueForm != "qualitative" {
		t.Fatalf("expected value_form qualitative, got %+v", p)
	}
	if p.NumericValue != nil || p.LowerValue != nil || p.UpperValue != nil {
		t.Fatalf("expected no numeric fields for qualitative, got %+v", p)
	}
	if p.AssertionKind != "observed_value" {
		t.Fatalf("expected qualitative observation to classify as observed_value, got %q", p.AssertionKind)
	}
}

func TestResolveMetricValueLimitAbsentHasNoNumericFields(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("limit_absent")
	r.ValueClass = ns("requirement")
	p := resolveMetricValueLegacy(r)
	if p.ValueForm != "limit_absent" {
		t.Fatalf("expected value_form limit_absent, got %+v", p)
	}
	if p.NumericValue != nil || p.LowerValue != nil || p.UpperValue != nil {
		t.Fatalf("expected no numeric fields for limit_absent, got %+v", p)
	}
}

func TestResolveMetricValueValueClassRefinesKind(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("lower_bound")
	r.ValueClass = ns("observation")
	r.MetricValue = ns("250")
	p := resolveMetricValueLegacy(r)
	if p.AssertionKind != "observed_value" {
		t.Fatalf("expected lower_bound + observation to classify as observed_value, got %q", p.AssertionKind)
	}
	if p.NumericValue == nil || *p.NumericValue != 250 {
		t.Fatalf("expected numeric value 250 preserved, got %+v", p.NumericValue)
	}
}

func TestResolveMetricValueStructuredLowerBoundNonNumericIsUnparsed(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("lower_bound")
	r.ValueClass = ns("requirement")
	r.MetricValue = ns("clearly legible")
	p := resolveMetricValueLegacy(r)
	if p.ValueForm != "unparsed" {
		t.Fatalf("expected structured lower_bound with non-numeric metric_value to be unparsed, got %+v", p)
	}
	if p.NumericValue != nil {
		t.Fatalf("expected no fabricated numeric value, got %+v", p.NumericValue)
	}
}

// TestResolveMetricValueLegacyFallsBackToParser locks in design D1: a row with
// NULL value_range_type (pre-structured-schema row) routes through
// parseThresholdOrTarget unchanged.
func TestResolveMetricValueLegacyFallsBackToParser(t *testing.T) {
	r := metricRowFixture()
	r.ThresholdOrTarget = ns("不低于250 cd/m²")
	p := resolveMetricValueLegacy(r)
	if p.ValueForm != "single" || p.Comparator != ">=" || p.AssertionKind != "lower_bound_requirement" {
		t.Fatalf("expected legacy row to fall back to the free-text parser, got %+v", p)
	}
	if p.NumericValue == nil || *p.NumericValue != 250 {
		t.Fatalf("expected numeric value 250, got %+v", p.NumericValue)
	}
}

func TestResolveMetricValueRangeMissingBoundsFallsBackHonestly(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("range")
	r.ValueClass = ns("requirement")
	r.MetricValue = ns("see table 3")
	// value_min/value_max NULL and no parseable range expression: this must
	// remain honest unparsed -- never a fabricated interval.
	p := resolveMetricValueLegacy(r)
	if p.ValueForm != "unparsed" {
		t.Fatalf("expected range row without value_min/value_max to be unparsed, got %+v", p)
	}
	if p.LowerValue != nil || p.UpperValue != nil {
		t.Fatalf("expected no fabricated bounds, got %+v", p)
	}
}

// TestNormalizeValueRangeTypeRawMatchesUnexported locks in that the exported
// wrapper (for callers outside this package, e.g. an admin correction
// handler keying kb.metric_value_range_type_map.raw_value) never drifts from
// the unexported function the runtime lookup actually uses.
func TestNormalizeValueRangeTypeRawMatchesUnexported(t *testing.T) {
	cases := []string{
		"", "  ", "Min", "MIN THRESHOLD", "at-least", "at least",
		"greater-than_or-equal to", "下限", "  Upper  Limit  ",
	}
	for _, raw := range cases {
		want := normalizeValueRangeTypeRaw(raw)
		got := NormalizeValueRangeTypeRaw(raw)
		if got != want {
			t.Fatalf("NormalizeValueRangeTypeRaw(%q) = %q, want %q (from normalizeValueRangeTypeRaw)", raw, got, want)
		}
	}
}

// TestInvalidateValueRangeTypeMapCacheClearsDefaultCache locks in that the
// exported wrapper actually reaches the same package-wide cache
// NewValueRangeTypeMapper hands to production code, not a copy.
func TestInvalidateValueRangeTypeMapCacheClearsDefaultCache(t *testing.T) {
	defaultValueRangeTypeMapCache.mu.Lock()
	defaultValueRangeTypeMapCache.entries = map[string]valueRangeTypeMapEntry{
		"min": {CanonicalBucket: "lower_bound", Status: "approved"},
	}
	defaultValueRangeTypeMapCache.mu.Unlock()

	InvalidateValueRangeTypeMapCache()

	defaultValueRangeTypeMapCache.mu.RLock()
	entries := defaultValueRangeTypeMapCache.entries
	defaultValueRangeTypeMapCache.mu.RUnlock()
	if entries != nil {
		t.Fatalf("expected defaultValueRangeTypeMapCache.entries to be nil after invalidate, got %+v", entries)
	}
}
