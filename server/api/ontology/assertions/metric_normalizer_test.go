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
