package assertions

import (
	"database/sql"
	"testing"
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

func TestResolveMetricValueLowerBound(t *testing.T) {
	r := metricRowFixture()
	r.ValueRangeType = ns("lower_bound")
	r.ValueClass = ns("requirement")
	r.MetricValue = ns("250")
	p := resolveMetricValue(r)
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
	p := resolveMetricValue(r)
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
	p := resolveMetricValue(r)
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

			p := resolveMetricValue(r)
			if p.Comparator != tt.comparator || p.AssertionKind != tt.kind {
				t.Fatalf("resolveMetricValue(%q) = %+v, want comparator=%q kind=%q", tt.rangeType, p, tt.comparator, tt.kind)
			}
			if p.NumericValue == nil || *p.NumericValue != tt.want {
				t.Fatalf("numeric value = %v, want %v", p.NumericValue, tt.want)
			}
		})
	}
}

func TestMetricCandidatePayloadCarriesMetricDefinitionTermID(t *testing.T) {
	r := metricRowFixture()
	r.MetricName = ns("Organic matter")
	r.MetricDefinitionTermID = ns("mea:organic_matter_mass_fraction")
	r.ValueRangeType = ns("min")
	r.ValueClass = ns("requirement")
	r.MetricValue = ns("≥30")

	payload := metricCandidatePayloadForRow(r)
	if got := payload["metric_definition_term_id"]; got != "mea:organic_matter_mass_fraction" {
		t.Fatalf("metric_definition_term_id = %#v, want resolved term id", got)
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
	p := resolveMetricValue(r)
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
	p := resolveMetricValue(r)
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
	p := resolveMetricValue(r)
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
	p := resolveMetricValue(r)
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
	p := resolveMetricValue(r)
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
	p := resolveMetricValue(r)
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
	r.MetricValue = ns("500:1 至 2000:1")
	// value_min/value_max NULL: endpoints can't come from structured columns,
	// and the spec forbids re-parsing free text for a range row, so this must
	// be honest unparsed -- not a fabricated interval from the text.
	p := resolveMetricValue(r)
	if p.ValueForm != "unparsed" {
		t.Fatalf("expected range row without value_min/value_max to be unparsed, got %+v", p)
	}
	if p.LowerValue != nil || p.UpperValue != nil {
		t.Fatalf("expected no fabricated bounds, got %+v", p)
	}
}
