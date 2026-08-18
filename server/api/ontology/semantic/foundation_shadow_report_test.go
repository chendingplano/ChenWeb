package semantic

import (
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/classfoundation"
)

func TestFoundationShadowReportExposesBoundedExceptionsAndClaimConvergence(t *testing.T) {
	report := NewFoundationShadowReport(1)
	report.Observe("metric-a", 7, MetricFoundationShadowResult{ClassState: MetricFoundationClassUnavailable}, 2)
	report.Observe("metric-b", 7, MetricFoundationShadowResult{
		ClassTermID:       "class:luminance",
		ClassState:        classfoundation.ResolutionResolvedExisting,
		CanonicalClaimKey: "claim-key",
		ProfileObservation: &classfoundation.ObservedProfileObservation{
			ObservationState: "missing",
		},
		Redirect: classfoundation.RedirectResolution{
			Status: classfoundation.RedirectResolved, Traversal: []string{"class:old", "class:luminance"}, TerminalTarget: "class:luminance",
		},
	}, 1)
	report.Observe("metric-c", 7, MetricFoundationShadowResult{
		ClassTermID: "class:luminance", ClassState: classfoundation.ResolutionResolvedExisting, CanonicalClaimKey: "claim-key",
	}, 1)
	report.Observe("metric-d", 7, MetricFoundationShadowResult{ClassState: MetricFoundationClassUnavailable}, 0)
	report.Finalize()

	if len(report.ClassExceptions) != 1 || len(report.DuplicateEvidenceExceptions) != 1 || len(report.ProfileOutliers) != 1 || len(report.Redirects) != 1 {
		t.Fatalf("exceptions = %#v", report)
	}
	if len(report.ClaimConvergences) != 1 || report.ClaimConvergences[0].CanonicalClaimKey != "claim-key" || report.ClaimConvergences[0].Occurrences != 2 {
		t.Fatalf("claim convergence = %#v", report.ClaimConvergences)
	}
	if !report.Truncated {
		t.Fatal("report must record truncation when its bounded detail limit is reached")
	}
}
