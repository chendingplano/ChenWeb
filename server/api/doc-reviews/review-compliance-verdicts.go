package docreviews

import (
	"fmt"
	"strings"
)

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

// complianceVerdictRows builds the per-candidate related_artifacts entries and the verdict section of
// the finding text. Verdict lines start with the verdict, so compliance-score's "- [type id] ...
// record N" pattern does not read them as reported passages. In a group, each line and summary
// names the activity it belongs to.
func complianceVerdictRows(verdicts []complianceVerdict, pool map[string]*complianceCandidate, u *complianceUnit) ([]RelatedArtifactAnalysis, string) {
	labels := map[string]string{}
	for _, m := range u.Members {
		labels[m.ID] = m.Label
	}
	group := len(u.Members) > 1
	ordered := orderComplianceVerdicts(verdicts)
	counts := map[string]int{}
	for _, v := range ordered {
		counts[v.Verdict]++
	}
	var b strings.Builder
	var countParts []string
	for _, verdict := range complianceVerdictOrder {
		if counts[verdict] > 0 {
			countParts = append(countParts, fmt.Sprintf("%d %s", counts[verdict], verdict))
		}
	}
	fmt.Fprintf(&b, "Candidate verdicts: %s\n", strings.Join(countParts, ", "))
	related := make([]RelatedArtifactAnalysis, 0, len(ordered))
	for _, v := range ordered {
		c := pool[v.ArtifactKey]
		if c == nil {
			continue
		}
		summary := v.Summary
		member := ""
		if group {
			member = fmt.Sprintf(" (activity %s: %s)", v.ActivityID, labels[v.ActivityID])
			summary = "[" + labels[v.ActivityID] + "] " + summary
		}
		fmt.Fprintf(&b, "%s [%s %s] record %d%s: %s\n", v.Verdict, c.Type, c.ID, c.RecordID, member, v.Summary)
		if v.Note != "" {
			fmt.Fprintf(&b, "  Note: %s\n", v.Note)
		}
		related = append(related, RelatedArtifactAnalysis{
			RelatedArtifactID: c.ID,
			RelatedRecordID:   c.RecordID,
			Relationship:      v.Verdict,
			Summary:           summary,
			Note:              v.Note,
		})
	}
	b.WriteString("\n")
	return related, b.String()
}
