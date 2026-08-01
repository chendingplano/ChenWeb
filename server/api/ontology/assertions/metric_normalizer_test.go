package assertions

import "testing"

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
