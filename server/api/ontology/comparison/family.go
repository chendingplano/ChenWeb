package comparison

// FamilyEvidence is what EvaluateFamily needs about one authority family's
// evidence for one metric on one target object, before a verdict can be
// decided. Representative selection among multiple matching assertions in a
// family (DR22 — precedence policy, supersession, newest non-superseded
// edition) happens before this call: EvaluateFamily always receives at most
// one already-chosen representative, never a list.
type FamilyEvidence struct {
	// Representative is the family's chosen constraint for this metric, or
	// nil if the family has no matching assertion at all.
	Representative *Constraint

	// DimensionClosed, when Representative is nil, decides between
	// StandardAbsent (the corpus was searched exhaustively for this
	// (metric, family) pair and truly has nothing) and Indeterminate
	// (evidence is incomplete). This is DR21 rule 2: absence alone never
	// licenses StandardAbsent.
	DimensionClosed bool

	// Applicable, when false, means the rule's stated condition excludes
	// this object regardless of what evidence exists or how the dimension
	// is declared. Checked first, so an inapplicable object never reports
	// StandardAbsent or Indeterminate about a requirement that was never
	// meant to apply to it.
	Applicable bool
}

// EvaluateFamily is the DR22 cell-level wrapper around Compare. Compare alone
// can only judge two present constraints; EvaluateFamily adds the
// absence/applicability outcomes that depend on review-scope declarations
// rather than on constraint content.
func EvaluateFamily(subject Constraint, ev FamilyEvidence) (Verdict, string, error) {
	if !ev.Applicable {
		return NotApplicable, "the rule's stated applicability condition excludes this object", nil
	}
	if ev.Representative == nil {
		if ev.DimensionClosed {
			return StandardAbsent, "no matching assertion in this family, and the dimension is declared closed (exhaustively searched)", nil
		}
		return Indeterminate, "no matching assertion in this family, and the dimension is not declared closed", nil
	}
	return Compare(subject, *ev.Representative)
}
