package semantic

import (
	"context"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/classfoundation"
)

type shadowRedirects map[string]string

func (r shadowRedirects) ActiveRedirect(_ context.Context, source string) (string, bool, error) {
	target, ok := r[source]
	return target, ok, nil
}

func TestMetricFoundationShadowComputesClassClaimProfileAndRedirectWithoutWriting(t *testing.T) {
	result, err := (MetricAdapter{}).FoundationShadow(context.Background(), MetricFoundationShadowInput{
		MetricID:             "metric-1",
		MetricDefinitionTerm: "class:legacy-luminance",
		MetricName:           "Display luminance",
		MetricValue:          "250",
		MetricUnit:           "cd/m2",
		RangeType:            "minimum",
		Redirects:            shadowRedirects{"class:legacy-luminance": "class:display-luminance"},
	})
	if err != nil {
		t.Fatalf("FoundationShadow: %v", err)
	}
	if result.ClassState != classfoundation.ResolutionResolvedExisting || result.ClassTermID != "class:display-luminance" {
		t.Fatalf("class result = %#v", result)
	}
	if result.CanonicalClaimKey == "" || result.ProfileObservation == nil {
		t.Fatalf("expected claim key and profile observation, got %#v", result)
	}
	if result.ProfileObservation.ClassTermID != "class:display-luminance" || result.ProfileObservation.ObservationState != "represented" {
		t.Fatalf("profile observation = %#v", result.ProfileObservation)
	}
	if result.Redirect.Status != classfoundation.RedirectResolved || result.Redirect.TerminalTarget != "class:display-luminance" {
		t.Fatalf("redirect = %#v", result.Redirect)
	}
}

func TestMetricFoundationShadowReportsMissingClassWithoutClaimOrProfile(t *testing.T) {
	result, err := (MetricAdapter{}).FoundationShadow(context.Background(), MetricFoundationShadowInput{
		MetricID: "metric-2", MetricName: "Unmapped metric",
	})
	if err != nil {
		t.Fatalf("FoundationShadow: %v", err)
	}
	if result.ClassState != MetricFoundationClassUnavailable || result.CanonicalClaimKey != "" || result.ProfileObservation != nil {
		t.Fatalf("missing class result = %#v", result)
	}
}
