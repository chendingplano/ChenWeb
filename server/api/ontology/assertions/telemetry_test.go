package assertions

import "testing"

// TestAssociationRunReportReconcilesByConstruction is spec §16.3 item 17 at
// unit-test granularity: the reconciliation invariant (every bucketed count
// sums back to ArtifactsExamined) holds for any report whose buckets were
// populated by the same row-by-row loop BuildAssociationRunReport uses. This
// exercises the invariant directly rather than only through a live-DB run.
func TestAssociationRunReportReconcilesByConstruction(t *testing.T) {
	r := AssociationRunReport{
		ArtifactsExamined: 3,
		CandidatesByMethod: map[string]int{
			"explicit_structured": 2,
			"human":               1,
		},
		ResolutionOutcomes: map[string]int{
			"matched":    1,
			"deferred":   1,
			"unresolved": 1,
		},
		LifecycleCounts: map[string]int{
			"accepted":  1,
			"deferred":  1,
			"candidate": 1,
		},
		DeferredByReason: map[string]int{"unresolved_referent": 1},
	}
	if !r.Reconciles() {
		t.Fatalf("expected reconciling report, got %+v", r)
	}
}

func TestAssociationRunReportDetectsUnaccountedCandidates(t *testing.T) {
	r := AssociationRunReport{
		ArtifactsExamined: 3,
		CandidatesByMethod: map[string]int{
			"explicit_structured": 2, // only 2, but 3 examined -- one unaccounted
		},
		ResolutionOutcomes: map[string]int{"matched": 3},
		LifecycleCounts:    map[string]int{"accepted": 3},
	}
	if r.Reconciles() {
		t.Fatal("expected a report with an undercounted bucket to fail reconciliation")
	}
}

func TestAssociationRunReportEmptyReconciles(t *testing.T) {
	r := AssociationRunReport{
		CandidatesByMethod: map[string]int{},
		ResolutionOutcomes: map[string]int{},
		LifecycleCounts:    map[string]int{},
	}
	if !r.Reconciles() {
		t.Fatal("expected an empty (zero-artifact) report to trivially reconcile")
	}
}
