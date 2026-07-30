package comparison

import "github.com/shopspring/decimal"

// ValueForm is the shape of a normalized constraint (research §6.2 value_form).
type ValueForm string

const (
	FormLowerBound  ValueForm = "lower_bound"
	FormUpperBound  ValueForm = "upper_bound"
	FormExactValue  ValueForm = "exact_value"
	FormRange       ValueForm = "range"
	FormQualitative ValueForm = "qualitative"  // a requirement exists but carries no decidable satisfying set
	FormLimitAbsent ValueForm = "limit_absent" // the property is referenced but no bound value is given
)

// Constraint is one normalized side of a comparison: a single metric
// assertion's value, already resolved to a property, quantity kind, and unit
// (DR9's normalized value columns), plus the optional Component qualifier
// that keeps independently-varying sub-values of one metric (e.g. horizontal
// vs vertical viewing angle) from ever being compared against each other by
// mistake.
//
// Which of Value / LowerValue / UpperValue is meaningful depends on Form:
// lower_bound and upper_bound use Value only; exact_value uses Value as the
// single permitted point; range uses LowerValue and UpperValue; qualitative
// and limit_absent use none of them.
type Constraint struct {
	QuantityKind string
	Unit         string
	Component    string

	Form ValueForm

	Value      decimal.Decimal
	LowerValue decimal.Decimal
	UpperValue decimal.Decimal
}

// bound is one endpoint of an interval. Value == nil means unbounded in that
// direction (lower bound: -infinity; upper bound: +infinity). Inclusive is
// meaningless when Value is nil.
type bound struct {
	Value     *decimal.Decimal
	Inclusive bool
}

// interval is the satisfying set of a Constraint, expressed in canonical
// units for its quantity kind so two constraints can be compared directly.
type interval struct {
	Lo bound
	Hi bound
}

func finite(v decimal.Decimal, inclusive bool) bound {
	return bound{Value: &v, Inclusive: inclusive}
}

var unbounded = bound{}

// buildInterval converts a Constraint's raw value(s) into a canonical-unit
// interval. It returns ok=false for qualitative and limit_absent forms,
// which have no satisfying set to build; callers must check those forms
// before calling buildInterval (Compare does this).
func buildInterval(c Constraint) (interval, bool, error) {
	switch c.Form {
	case FormLowerBound:
		v, err := toCanonical(c.QuantityKind, c.Unit, c.Value)
		if err != nil {
			return interval{}, false, err
		}
		return interval{Lo: finite(v, true), Hi: unbounded}, true, nil
	case FormUpperBound:
		v, err := toCanonical(c.QuantityKind, c.Unit, c.Value)
		if err != nil {
			return interval{}, false, err
		}
		return interval{Lo: unbounded, Hi: finite(v, true)}, true, nil
	case FormExactValue:
		v, err := toCanonical(c.QuantityKind, c.Unit, c.Value)
		if err != nil {
			return interval{}, false, err
		}
		return interval{Lo: finite(v, true), Hi: finite(v, true)}, true, nil
	case FormRange:
		lo, err := toCanonical(c.QuantityKind, c.Unit, c.LowerValue)
		if err != nil {
			return interval{}, false, err
		}
		hi, err := toCanonical(c.QuantityKind, c.Unit, c.UpperValue)
		if err != nil {
			return interval{}, false, err
		}
		return interval{Lo: finite(lo, true), Hi: finite(hi, true)}, true, nil
	case FormQualitative, FormLimitAbsent:
		return interval{}, false, nil
	default:
		return interval{}, false, nil
	}
}

// lowerLE reports whether bound a's lower threshold is at least as permissive
// as bound b's — i.e. every value b's lower bound admits, a's lower bound
// also admits. -infinity (nil) is at least as permissive as anything.
//
// At equal values, an inclusive bound is more permissive than an exclusive
// one (it admits one more point, the boundary itself), which is why equal
// values still need the tie-break below rather than stopping at Cmp==0.
func lowerLE(a, b bound) bool {
	if a.Value == nil {
		return true
	}
	if b.Value == nil {
		return false
	}
	switch a.Value.Cmp(*b.Value) {
	case -1:
		return true
	case 1:
		return false
	default:
		return a.Inclusive || !b.Inclusive
	}
}

// upperGE mirrors lowerLE for upper thresholds; +infinity (nil) is at least
// as permissive as anything.
func upperGE(a, b bound) bool {
	if a.Value == nil {
		return true
	}
	if b.Value == nil {
		return false
	}
	switch a.Value.Cmp(*b.Value) {
	case 1:
		return true
	case -1:
		return false
	default:
		return a.Inclusive || !b.Inclusive
	}
}

// contains reports whether inner's satisfying set is a subset of outer's.
func contains(outer, inner interval) bool {
	return lowerLE(outer.Lo, inner.Lo) && upperGE(outer.Hi, inner.Hi)
}

func equalInterval(a, b interval) bool {
	return contains(a, b) && contains(b, a)
}

// upperBeforeLower reports whether upper bound u ends strictly before lower
// bound l begins, i.e. no value can satisfy both a constraint ending at u and
// one beginning at l. At equal values the two bounds still overlap (share
// that one point) only if both are inclusive.
func upperBeforeLower(u, l bound) bool {
	if u.Value == nil || l.Value == nil {
		return false // +infinity never precedes anything; nothing precedes -infinity
	}
	switch u.Value.Cmp(*l.Value) {
	case -1:
		return true
	case 1:
		return false
	default:
		return !(u.Inclusive && l.Inclusive)
	}
}

func disjoint(a, b interval) bool {
	return upperBeforeLower(a.Hi, b.Lo) || upperBeforeLower(b.Hi, a.Lo)
}
