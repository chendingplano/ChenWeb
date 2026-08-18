package classfoundation

import "testing"

func TestCapsFromEnvUsesSafeDefaultsAndPositiveOverrides(t *testing.T) {
	t.Setenv("ONTOLOGY_PROFILE_MAX_EXAMPLES", "")
	t.Setenv("ONTOLOGY_REDIRECT_MAX_DEPTH", "0")
	t.Setenv("ONTOLOGY_CLAIM_SHADOW_REPORT_MAX_ROWS", "bad")
	t.Setenv("ONTOLOGY_REVIEW_SAME_CLASS_MAX_RESULTS", "")

	if got := CapsFromEnv(); got != (Caps{ProfileExamples: 25, RedirectDepth: 16, ClaimShadowReportRows: 1000, ReviewSameClassResults: 20}) {
		t.Fatalf("default caps = %+v", got)
	}

	t.Setenv("ONTOLOGY_PROFILE_MAX_EXAMPLES", "3")
	t.Setenv("ONTOLOGY_REDIRECT_MAX_DEPTH", "4")
	t.Setenv("ONTOLOGY_CLAIM_SHADOW_REPORT_MAX_ROWS", "500")
	t.Setenv("ONTOLOGY_REVIEW_SAME_CLASS_MAX_RESULTS", "12")
	if got := CapsFromEnv(); got != (Caps{ProfileExamples: 3, RedirectDepth: 4, ClaimShadowReportRows: 500, ReviewSameClassResults: 12}) {
		t.Fatalf("override caps = %+v", got)
	}
}
