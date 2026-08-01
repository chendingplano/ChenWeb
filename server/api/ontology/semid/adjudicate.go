package semid

// Verdict is the kernel's adjudication outcome for one surface.
type Verdict string

const (
	VerdictAutoAccept  Verdict = "auto_accepted"
	VerdictAmbiguous   Verdict = "ambiguous"
	VerdictDeferred    Verdict = "deferred"
	VerdictHumanReview Verdict = "human_review"
)

// AutoAcceptPolicy is the family's gate for automatic acceptance. Governed
// families (ontology terms) disable it, so adjudication always ends at a
// reviewable change set (DR15: "governed, so adjudication ends at a change
// set rather than an auto-accept").
type AutoAcceptPolicy struct {
	Enabled      bool
	MinScore     float64
	MaxCandidates int
}

// Adjudicate decides the outcome given the scored matches (sorted best-first)
// and the family's auto-accept policy:
//
//   - no candidates          -> deferred
//   - top score tied across multiple candidates -> ambiguous
//   - auto-accept disabled, or best below MinScore -> human_review
//   - otherwise              -> auto_accepted
func Adjudicate(matches []ScoredMatch, policy AutoAcceptPolicy) Verdict {
	if len(matches) == 0 {
		return VerdictDeferred
	}
	top := matches[0].Score
	tied := 0
	for _, m := range matches {
		if m.Score == top {
			tied++
		}
	}
	if policy.MaxCandidates > 0 && tied > policy.MaxCandidates {
		return VerdictAmbiguous
	}
	if !policy.Enabled || top < policy.MinScore {
		return VerdictHumanReview
	}
	return VerdictAutoAccept
}
