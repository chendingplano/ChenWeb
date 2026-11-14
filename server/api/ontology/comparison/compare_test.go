package comparison

import (
	"testing"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func lowerBound(quantityKind, unit, value string) Constraint {
	return Constraint{QuantityKind: quantityKind, Unit: unit, Form: FormLowerBound, Value: dec(value)}
}

func upperBound(quantityKind, unit, value string) Constraint {
	return Constraint{QuantityKind: quantityKind, Unit: unit, Form: FormUpperBound, Value: dec(value)}
}

func exactValue(quantityKind, unit, value string) Constraint {
	return Constraint{QuantityKind: quantityKind, Unit: unit, Form: FormExactValue, Value: dec(value)}
}

func rangeValue(quantityKind, unit, lo, hi string) Constraint {
	return Constraint{QuantityKind: quantityKind, Unit: unit, Form: FormRange, LowerValue: dec(lo), UpperValue: dec(hi)}
}

func qualitative(quantityKind string) Constraint {
	return Constraint{QuantityKind: quantityKind, Form: FormQualitative}
}

func limitAbsent(quantityKind string) Constraint {
	return Constraint{QuantityKind: quantityKind, Form: FormLimitAbsent}
}

func assertVerdict(t *testing.T, subject, reference Constraint, want Verdict) {
	t.Helper()
	got, rationale, err := Compare(subject, reference)
	if err != nil {
		t.Fatalf("Compare returned error: %v", err)
	}
	if got != want {
		t.Fatalf("Compare = %q (%s), want %q", got, rationale, want)
	}
	if rationale == "" {
		t.Fatalf("Compare returned an empty rationale for verdict %q", got)
	}
}

func TestCompareIdenticalSameUnit(t *testing.T) {
	assertVerdict(t, upperBound("Time", "ms", "120"), upperBound("Time", "ms", "120"), Identical)
}

func TestCompareEquivalentAfterUnitConversion(t *testing.T) {
	// 0.12 s == 120 ms.
	assertVerdict(t, upperBound("Time", "ms", "120"), upperBound("Time", "s", "0.12"), Equivalent)
}

func TestCompareEquivalentRatioUnitAliasOne(t *testing.T) {
	// Real extract_metrics output has used "1" as the unit string for a
	// ratio value (e.g. "1000:1" contrast) where other documents used
	// "ratio" for the identical concept; "1" must be treated as an alias,
	// not a distinct unit, so these still compare as Equivalent.
	assertVerdict(t, lowerBound("Dimensionless", "ratio", "1000"), lowerBound("Dimensionless", "1", "1000"), Equivalent)
}

func TestCompareStrongerTighterUpperBound(t *testing.T) {
	// subject <=120ms is a strict subset of reference <=150ms: subject is stronger.
	assertVerdict(t, upperBound("Time", "ms", "120"), upperBound("Time", "ms", "150"), Stronger)
}

func TestCompareWeakerLooserLowerBound(t *testing.T) {
	// subject >=300 strictly contains reference >=500: subject is weaker.
	assertVerdict(t, lowerBound("Count", "count", "300"), lowerBound("Count", "count", "500"), Weaker)
}

func TestCompareConflictExactValueOutsideUpperBound(t *testing.T) {
	assertVerdict(t, upperBound("Time", "ms", "120"), exactValue("Time", "ms", "200"), Conflict)
}

func TestCompareIncomparableOverlapWithoutContainment(t *testing.T) {
	// subject >=1000 overlaps reference [500,2000] on [1000,2000]; neither contains the other.
	assertVerdict(t, lowerBound("Dimensionless", "ratio", "1000"), rangeValue("Dimensionless", "ratio", "500", "2000"), Incomparable)
}

func TestCompareQualitativeOnlyEitherSide(t *testing.T) {
	assertVerdict(t, lowerBound("Luminance", "cd/m2", "250"), qualitative("Luminance"), QualitativeOnly)
	assertVerdict(t, qualitative("Luminance"), lowerBound("Luminance", "cd/m2", "250"), QualitativeOnly)
}

func TestCompareLimitAbsentEitherSide(t *testing.T) {
	assertVerdict(t, lowerBound("Dimensionless", "px_pair", "614400"), limitAbsent("Dimensionless"), LimitAbsent)
}

func TestCompareBoundaryInclusiveExactValueEqualsUpperBound(t *testing.T) {
	// exact_value{120} is a subset of upper_bound<=120 (equal at the boundary,
	// both inclusive): {120} subset (-inf,120], and {120} != (-inf,120], so
	// the exact-value side (as reference) is not itself a superset -- subject
	// (the bound) is weaker than the exact-value pin.
	assertVerdict(t, upperBound("Time", "ms", "120"), exactValue("Time", "ms", "120"), Weaker)
}

func TestCompareQuantityKindMismatchRecordsNoVerdict(t *testing.T) {
	assertVerdict(t, upperBound("Time", "ms", "120"), upperBound("Luminance", "cd/m2", "250"), NoVerdict)
}

func TestCompareComponentMismatchRecordsNoVerdict(t *testing.T) {
	subject := lowerBound("Angle", "degree", "160")
	subject.Component = "horizontal"
	reference := lowerBound("Angle", "degree", "140")
	reference.Component = "vertical"
	assertVerdict(t, subject, reference, NoVerdict)
}

func TestCompareUnknownUnitRecordsNoVerdict(t *testing.T) {
	assertVerdict(t, upperBound("Time", "fortnight", "1"), upperBound("Time", "ms", "120"), NoVerdict)
}

func TestCompareLengthUnits(t *testing.T) {
	assertVerdict(t, exactValue("Length", "cm", "0.5"), lowerBound("Length", "mm", "5"), Stronger)
	assertVerdict(t, exactValue("Length", "m", "0.005"), exactValue("Length", "mm", "5"), Equivalent)
	strict := lowerBound("Length", "mm", "5")
	strict.LowerExclusive = true
	assertVerdict(t, strict, exactValue("Length", "cm", "0.5"), Conflict)
}

func TestComparePercentUnits(t *testing.T) {
	for _, unit := range []string{"percent", "%"} {
		t.Run(unit, func(t *testing.T) {
			assertVerdict(t, exactValue("Dimensionless", unit, "50"), exactValue("Dimensionless", "ratio", "0.5"), Equivalent)
		})
	}
}

func TestCompareUnsupportedAndMismatchedUnits(t *testing.T) {
	assertVerdict(t, exactValue("Length", "mm", "5"), exactValue("Time", "ms", "5"), NoVerdict)
	assertVerdict(t, exactValue("Length", "ms", "5"), exactValue("Length", "mm", "5"), NoVerdict)
	assertVerdict(t, exactValue("Length", "inch", "5"), exactValue("Length", "mm", "5"), NoVerdict)
	assertVerdict(t, exactValue("Time", "month", "1"), exactValue("Time", "day", "30"), NoVerdict)
}

func TestCompareReversedRangeHasNoVerdict(t *testing.T) {
	invalid := rangeValue("Time", "ms", "10", "5")
	valid := rangeValue("Time", "ms", "0", "20")
	assertVerdict(t, invalid, valid, NoVerdict)
	assertVerdict(t, valid, invalid, NoVerdict)
}

func TestCompareExclusiveBounds(t *testing.T) {
	lower := lowerBound("Time", "ms", "5")
	lower.LowerExclusive = true
	upper := upperBound("Time", "ms", "5")
	upper.UpperExclusive = true
	for _, strict := range []Constraint{lower, upper} {
		assertVerdict(t, strict, exactValue("Time", "ms", "5"), Conflict)
	}
	assertVerdict(t, lower, lowerBound("Time", "ms", "5"), Stronger)
	assertVerdict(t, upper, upperBound("Time", "ms", "5"), Stronger)
}

func TestCompareExclusiveRangeEndpoints(t *testing.T) {
	for _, endpoint := range []string{"5", "10"} {
		t.Run(endpoint, func(t *testing.T) {
			open := rangeValue("Time", "ms", "5", "10")
			open.LowerExclusive = endpoint == "5"
			open.UpperExclusive = endpoint == "10"
			assertVerdict(t, open, exactValue("Time", "ms", endpoint), Conflict)
			assertVerdict(t, open, rangeValue("Time", "ms", "5", "10"), Stronger)
			assertVerdict(t, exactValue("Time", "ms", "7"), open, Stronger)
		})
	}
}

func TestCompareEmptyRangeHasNoVerdict(t *testing.T) {
	for _, flags := range [][2]bool{{true, false}, {false, true}, {true, true}} {
		empty := rangeValue("Time", "ms", "5", "5")
		empty.LowerExclusive, empty.UpperExclusive = flags[0], flags[1]
		point := exactValue("Time", "ms", "5")
		assertVerdict(t, empty, point, NoVerdict)
		assertVerdict(t, point, empty, NoVerdict)
	}
	assertVerdict(t, rangeValue("Time", "ms", "5", "5"), exactValue("Time", "ms", "5"), Identical)
}

func TestEvaluateFamilyNotApplicableTakesPrecedence(t *testing.T) {
	subject := lowerBound("Luminance", "cd/m2", "250")
	rep := upperBound("Luminance", "cd/m2", "999") // would otherwise compare fine
	got, _, err := EvaluateFamily(subject, FamilyEvidence{Representative: &rep, DimensionClosed: true, Applicable: false})
	if err != nil {
		t.Fatalf("EvaluateFamily returned error: %v", err)
	}
	if got != NotApplicable {
		t.Fatalf("EvaluateFamily = %q, want %q", got, NotApplicable)
	}
}

func TestEvaluateFamilyStandardAbsentWhenClosedAndNoEvidence(t *testing.T) {
	subject := lowerBound("Luminance", "cd/m2", "250")
	got, _, err := EvaluateFamily(subject, FamilyEvidence{Representative: nil, DimensionClosed: true, Applicable: true})
	if err != nil {
		t.Fatalf("EvaluateFamily returned error: %v", err)
	}
	if got != StandardAbsent {
		t.Fatalf("EvaluateFamily = %q, want %q", got, StandardAbsent)
	}
}

func TestEvaluateFamilyIndeterminateWhenOpenAndNoEvidence(t *testing.T) {
	subject := lowerBound("Angle", "degree", "160")
	got, _, err := EvaluateFamily(subject, FamilyEvidence{Representative: nil, DimensionClosed: false, Applicable: true})
	if err != nil {
		t.Fatalf("EvaluateFamily returned error: %v", err)
	}
	if got != Indeterminate {
		t.Fatalf("EvaluateFamily = %q, want %q", got, Indeterminate)
	}
}

func TestEvaluateFamilyDelegatesToCompareWhenEvidencePresent(t *testing.T) {
	subject := upperBound("Time", "ms", "120")
	rep := upperBound("Time", "ms", "120")
	got, _, err := EvaluateFamily(subject, FamilyEvidence{Representative: &rep, Applicable: true})
	if err != nil {
		t.Fatalf("EvaluateFamily returned error: %v", err)
	}
	if got != Identical {
		t.Fatalf("EvaluateFamily = %q, want %q", got, Identical)
	}
}
