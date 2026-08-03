package profiles

import (
	"testing"
)

// P5ProfilesAcceptanceCriteria maps the profile-related acceptance criteria
// (9, 10, 14) from spec 2026080102 section 12 to named tests in this package.
// The doc-processing package's p5_exit_test.go references these pointers.
var P5ProfilesAcceptanceCriteria = map[int][]string{
	// 9. review-profile selection evaluates identical predicates
	9: {
		"TestReviewProfileSelectionIdenticalPredicates",
		"TestReviewProfileReleasePinsSurviveActivation",
	},
	// 10. deterministic scope creation
	10: {
		"TestDeterministicScopeCreationOneKnowledgeStore",
		"TestScopeCreationAtomicPinsAndSnapshots",
		"TestScopeCreationExplicitIndeterminateContinuation",
		"TestScopeCreationLeavesExplicitP4ScopesByteCompatible",
	},
	// 14. review snapshots reproducible after activation changes
	14: {
		"TestReviewReloadAfterActivationChange",
	},
}

// TestP5ProfilesAcceptanceCriteriaCoverage fails if any profile-related
// criterion lacks at least one named test.
func TestP5ProfilesAcceptanceCriteriaCoverage(t *testing.T) {
	for _, criterion := range []int{9, 10, 14} {
		tests, ok := P5ProfilesAcceptanceCriteria[criterion]
		if !ok || len(tests) == 0 {
			t.Errorf("criterion %d lacks named test in profiles package", criterion)
		}
	}
}
