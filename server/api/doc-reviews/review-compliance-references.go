package docreviews

import (
	"fmt"
	"sort"
	"strings"
)

// A reference is resolved only within the reviewed document's evidence boundary.
// Exact quotation checks establish provenance; the semantic verifier still checks
// that the cited passage actually resolves the reference.
type complianceReferenceResolution struct {
	Reference string               `json:"reference"`
	Resolved  bool                 `json:"resolved"`
	Evidence  []complianceEvidence `json:"evidence"`
}

func validateComplianceReferences(results []complianceAssessment, screenings []complianceScreening, docs map[int64]complianceDocument, reviewedRecordID int64) error {
	wanted := map[string]map[string]bool{}
	for _, screen := range screenings {
		if wanted[screen.RequirementID] == nil {
			wanted[screen.RequirementID] = map[string]bool{}
		}
		for _, reference := range screen.UnresolvedReferences {
			reference = strings.TrimSpace(reference)
			if reference == "" {
				return fmt.Errorf("compliance: empty unresolved reference for %s", screen.RequirementID)
			}
			wanted[screen.RequirementID][reference] = true
		}
	}
	for i := range results {
		result := &results[i]
		resolved := map[string]bool{}
		seen := map[string]bool{}
		for _, resolution := range result.References {
			reference := strings.TrimSpace(resolution.Reference)
			if !wanted[result.RequirementID][reference] || seen[reference] {
				return fmt.Errorf("compliance: unknown or duplicate reference resolution %q for %s", reference, result.RequirementID)
			}
			seen[reference] = true
			if resolution.Resolved && len(resolution.Evidence) == 0 {
				return fmt.Errorf("compliance: resolved reference %q needs canonical reviewed-document evidence", reference)
			}
			for _, evidence := range resolution.Evidence {
				if evidence.RecordID != reviewedRecordID {
					return fmt.Errorf("compliance: reference %q cites evidence outside the reviewed document", reference)
				}
				if err := verifyComplianceEvidence(evidence, docs); err != nil {
					return err
				}
			}
			resolved[reference] = resolution.Resolved
		}
		var unresolved []string
		for reference := range wanted[result.RequirementID] {
			if !resolved[reference] {
				unresolved = append(unresolved, reference)
			}
		}
		if len(unresolved) > 0 && complianceOneOf(result.Status, "supported", "contradicted", "partially_supported", "not_documented") {
			sort.Strings(unresolved)
			result.Status = "insufficient_evidence"
			result.Suggestion = ""
			result.Rationale = strings.TrimSpace(result.Rationale + " Material references remain unresolved within the reviewed document: " + strings.Join(unresolved, "; ") + ".")
		}
	}
	return nil
}
