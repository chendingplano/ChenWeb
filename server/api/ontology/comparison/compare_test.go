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
