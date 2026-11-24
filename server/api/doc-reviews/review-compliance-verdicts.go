package docreviews

// Candidate verdicts: every candidate that passes the relevance check gets one verdict and a summary
// of how it relates to its activity (devdoc 2026111201, Candidate verdicts).
const (
	verdictCompliant    = "compliant"
	verdictCompatible   = "compatible"
	verdictNotCompliant = "not_compliant"
	verdictUndetermined = "undetermined"
	verdictUnqualified  = "unqualified"
)

// complianceVerdictOrder is the display order: actionable verdicts first, droppable last.
var complianceVerdictOrder = []string{verdictNotCompliant, verdictCompatible, verdictCompliant, verdictUndetermined, verdictUnqualified}

// orderComplianceVerdicts returns the verdicts sorted for display, keeping their order within one verdict.
func orderComplianceVerdicts(verdicts []complianceVerdict) []complianceVerdict {
	out := make([]complianceVerdict, 0, len(verdicts))
	for _, verdict := range complianceVerdictOrder {
		for _, v := range verdicts {
			if v.Verdict == verdict {
				out = append(out, v)
			}
		}
	}
	return out
}
