package comparison

import (
	"fmt"
	"strings"
)

// Verdict is one DR21 comparison result. Compare returns only the first
// eight; StandardAbsent, NotApplicable, and Indeterminate are returned by
// EvaluateFamily, since they depend on evidence absence and applicability
// rather than on comparing two present constraints.
type Verdict string

const (
	Identical       Verdict = "identical"
	Equivalent      Verdict = "equivalent"
	Stronger        Verdict = "stronger"
	Weaker          Verdict = "weaker"
	Conflict        Verdict = "conflict"
	Incomparable    Verdict = "incomparable"
	QualitativeOnly Verdict = "qualitative_only"
	LimitAbsent     Verdict = "limit_absent"
	StandardAbsent  Verdict = "standard_absent"
	NotApplicable   Verdict = "not_applicable"
	Indeterminate   Verdict = "indeterminate"
	NoVerdict       Verdict = "no_verdict"
)

// Compare implements the DR21 strictness relation between a subject
// constraint and a reference constraint. Per DR22, `subject` is always the
// enterprise/subject-organization side and `reference` is one authority
// family's constraint; Stronger/Weaker are reported relative to subject
// (Stronger means subject's requirement is the tighter one).
//
// Compare never returns an error for a qualitative or limit_absent side —
// those are legitimate DR21 outcomes, checked first, before any unit or
// quantity-kind validation that a genuinely non-comparable side wouldn't
// satisfy anyway.
func Compare(subject, reference Constraint) (Verdict, string, error) {
	if subject.Form == FormQualitative || reference.Form == FormQualitative {
		return QualitativeOnly, "a compared side states a requirement with no decidable satisfying set", nil
	}
	if subject.Form == FormLimitAbsent || reference.Form == FormLimitAbsent {
		return LimitAbsent, "the property is referenced but no bound value is given", nil
	}
	if subject.QuantityKind != reference.QuantityKind {
		return NoVerdict, fmt.Sprintf(
			"cannot compare different quantity kinds: subject %q vs reference %q", subject.QuantityKind, reference.QuantityKind), nil
	}
	if subject.Component != reference.Component {
		return NoVerdict, fmt.Sprintf(
			"cannot compare different components: subject %q vs reference %q", subject.Component, reference.Component), nil
	}

	sIv, ok, err := buildInterval(subject)
	if err != nil {
		return NoVerdict, "subject normalization unavailable: " + err.Error(), nil
	}
	if !ok {
		return "", "", fmt.Errorf("comparison: subject form %q has no satisfying set", subject.Form)
	}
	rIv, ok, err := buildInterval(reference)
	if err != nil {
		return NoVerdict, "reference normalization unavailable: " + err.Error(), nil
	}
	if !ok {
		return "", "", fmt.Errorf("comparison: reference form %q has no satisfying set", reference.Form)
	}

	sameUnit := strings.EqualFold(subject.Unit, reference.Unit)

	switch {
	case equalInterval(sIv, rIv):
		if sameUnit {
			return Identical, "equal satisfying sets, same unit", nil
		}
		return Equivalent, "equal satisfying sets after unit conversion, different unit expression", nil
	case contains(rIv, sIv):
		return Stronger, "subject's satisfying set is a strict subset of reference's: subject is the tighter requirement", nil
	case contains(sIv, rIv):
		return Weaker, "reference's satisfying set is a strict subset of subject's: subject is the looser requirement", nil
	case disjoint(sIv, rIv):
		return Conflict, "the satisfying sets do not overlap", nil
	default:
		return Incomparable, "the satisfying sets overlap without either containing the other", nil
	}
}
