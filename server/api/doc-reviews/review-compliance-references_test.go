package docreviews

import (
	"strings"
	"testing"
)

func complianceReferenceTestDocument() map[int64]complianceDocument {
	return map[int64]complianceDocument{7: {RecordID: 7, Lines: []complianceInputLine{{LineNumber: 250, Content: "Annex A: retain logs for 90 days."}}}}
}

func TestComplianceReferenceResolvedInLaterChunk(t *testing.T) {
	anchor := complianceEvidence{RecordID: 7, LineStart: 250, LineEnd: 250, Quote: "Annex A: retain logs for 90 days."}
	results := []complianceAssessment{{RequirementID: "r1", Status: "supported", References: []complianceReferenceResolution{{Reference: "Annex A", Resolved: true, Evidence: []complianceEvidence{anchor}}}}}
	screens := []complianceScreening{{RequirementID: "r1", UnresolvedReferences: []string{"Annex A"}}, {RequirementID: "r1", Evidence: []complianceEvidence{anchor}, UnresolvedReferences: []string{" Annex A "}}}
	if err := validateComplianceReferences(results, screens, complianceReferenceTestDocument(), 7); err != nil {
		t.Fatal(err)
	}
	if results[0].Status != "supported" {
		t.Fatalf("resolved reference suppressed conclusion: %+v", results)
	}
}

func TestComplianceUnresolvedReferenceOnlyDowngradesAffectedRequirement(t *testing.T) {
	for _, status := range []string{"supported", "contradicted", "partially_supported", "not_documented"} {
		for _, omitted := range []bool{false, true} {
			results := []complianceAssessment{{RequirementID: "r1", Status: status, Rationale: "Draft evidence assessment.", Suggestion: "Revise specification."}, {RequirementID: "r2", Status: "supported", Suggestion: "Unaffected."}}
			if !omitted {
				results[0].References = []complianceReferenceResolution{{Reference: "External retention policy", Resolved: false}}
			}
			screens := []complianceScreening{{RequirementID: "r1", UnresolvedReferences: []string{"External retention policy"}}, {RequirementID: "r2"}}
			if err := validateComplianceReferences(results, screens, complianceReferenceTestDocument(), 7); err != nil {
				t.Fatal(err)
			}
			if results[0].Status != "insufficient_evidence" || results[0].Suggestion != "" || !strings.Contains(results[0].Rationale, "External retention policy") {
				t.Fatalf("unresolved reference accepted: %+v", results[0])
			}
			if results[1].Status != "supported" || results[1].Suggestion != "Unaffected." {
				t.Fatalf("unrelated requirement changed: %+v", results[1])
			}
		}
	}
}

func TestComplianceResolvedReferenceRequiresOriginalTargetEvidence(t *testing.T) {
	valid := complianceEvidence{RecordID: 7, LineStart: 250, LineEnd: 250, Quote: "Annex A: retain logs for 90 days."}
	fake := valid
	fake.Quote = "Annex A: retain logs for 900 days."
	other := valid
	other.RecordID = 9
	for _, evidence := range [][]complianceEvidence{nil, {fake}, {other}} {
		results := []complianceAssessment{{RequirementID: "r1", Status: "supported", References: []complianceReferenceResolution{{Reference: "Annex A", Resolved: true, Evidence: evidence}}}}
		if err := validateComplianceReferences(results, []complianceScreening{{RequirementID: "r1", UnresolvedReferences: []string{"Annex A"}}}, complianceReferenceTestDocument(), 7); err == nil {
			t.Fatalf("invalid resolution accepted: %+v", evidence)
		}
	}
}

func TestComplianceReferenceRejectsUnknownOrDuplicateResolutions(t *testing.T) {
	for _, refs := range [][]complianceReferenceResolution{
		{{Reference: "Unknown annex", Resolved: false}},
		{{Reference: "Annex A", Resolved: false}, {Reference: " Annex A ", Resolved: false}},
	} {
		results := []complianceAssessment{{RequirementID: "r1", Status: "supported", References: refs}}
		if err := validateComplianceReferences(results, []complianceScreening{{RequirementID: "r1", UnresolvedReferences: []string{"Annex A"}}}, complianceReferenceTestDocument(), 7); err == nil {
			t.Fatalf("invalid references accepted: %+v", refs)
		}
	}
}

func TestComplianceUnresolvedReferencePreservesAbstention(t *testing.T) {
	results := []complianceAssessment{{RequirementID: "r1", Status: "not_assessed", Rationale: "Applicability unknown."}}
	if err := validateComplianceReferences(results, []complianceScreening{{RequirementID: "r1", UnresolvedReferences: []string{"Annex A"}}}, complianceReferenceTestDocument(), 7); err != nil {
		t.Fatal(err)
	}
	if results[0].Status != "not_assessed" || results[0].Rationale != "Applicability unknown." {
		t.Fatalf("abstention changed: %+v", results)
	}
}

func TestComplianceNoReferencesNeedsNoResolution(t *testing.T) {
	results := []complianceAssessment{{RequirementID: "r1", Status: "not_documented", Rationale: "Complete screening found no specification."}}
	if err := validateComplianceReferences(results, []complianceScreening{{RequirementID: "r1"}}, nil, 7); err != nil {
		t.Fatal(err)
	}
	if results[0].Status != "not_documented" {
		t.Fatalf("reference-free result changed: %+v", results)
	}
}
