package semantic

import (
	"context"
	"errors"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/ontology/classfoundation"
)

// MetricFoundationClassUnavailable records the deliberate shadow outcome for
// a metric with no existing source-backed class candidate. Shadow mode must
// report this gap rather than create a provisional class or a claim.
const MetricFoundationClassUnavailable = "source_class_unavailable"

// MetricFoundationShadowInput is the source-backed metric shape used to
// compute the class, claim, profile, and redirect views. It is intentionally
// separate from persistence: the Phase 3 writer will own creation of classes,
// claims, assertions, evidence, and profile rows.
type MetricFoundationShadowInput struct {
	MetricID             string
	MetricDefinitionTerm string
	MetricName           string
	MetricValue          string
	ThresholdOrTarget    string
	MetricUnit           string
	RangeType            string
	Redirects            classfoundation.RedirectLookup
}

// MetricFoundationShadowResult is a purely computed foundation view for one
// metric. CanonicalClaimKey is not registered and ProfileObservation is not
// recorded; both are reportable shadow candidates only.
type MetricFoundationShadowResult struct {
	ClassTermID        string
	ClassState         string
	CanonicalClaimKey  string
	ProfileObservation *classfoundation.ObservedProfileObservation
	Redirect           classfoundation.RedirectResolution
}

// FoundationShadow connects the current metric source shape to all four ADR
// foundations without writing. A source-backed class candidate is redirected
// when a redirect lookup is supplied; unavailable or unresolved classes remain
// explicit gaps and cannot produce a claim/profile candidate.
func (MetricAdapter) FoundationShadow(ctx context.Context, input MetricFoundationShadowInput) (MetricFoundationShadowResult, error) {
	if strings.TrimSpace(input.MetricID) == "" {
		return MetricFoundationShadowResult{}, errors.New("metric ID is required")
	}
	classTermID := strings.TrimSpace(input.MetricDefinitionTerm)
	if classTermID == "" {
		return MetricFoundationShadowResult{ClassState: MetricFoundationClassUnavailable}, nil
	}

	redirect := classfoundation.RedirectResolution{
		TerminalTarget: classTermID,
		Traversal:      []string{classTermID},
		Status:         classfoundation.RedirectResolved,
	}
	if input.Redirects != nil {
		redirect = (classfoundation.RedirectResolver{Lookup: input.Redirects}).Resolve(ctx, classTermID)
	}
	if redirect.Status != classfoundation.RedirectResolved || strings.TrimSpace(redirect.TerminalTarget) == "" {
		return MetricFoundationShadowResult{
			ClassState: MetricFoundationClassUnavailable,
			Redirect:   redirect,
		}, nil
	}
	classTermID = redirect.TerminalTarget
	rawValue := strings.TrimSpace(input.MetricValue)
	if rawValue == "" {
		rawValue = strings.TrimSpace(input.ThresholdOrTarget)
	}
	observationState := "represented"
	if rawValue == "" {
		observationState = "missing"
	}
	claimKey, err := classfoundation.SerializeCanonicalClaimKey(classfoundation.CanonicalClaimInput{
		KeyVersion:  "identity/v1",
		ClassTermID: classTermID,
		Fields: map[string]string{
			"metric_name":  strings.TrimSpace(input.MetricName),
			"metric_unit":  strings.TrimSpace(input.MetricUnit),
			"metric_value": rawValue,
			"range_type":   strings.TrimSpace(input.RangeType),
		},
	})
	if err != nil {
		return MetricFoundationShadowResult{}, err
	}
	return MetricFoundationShadowResult{
		ClassTermID:       classTermID,
		ClassState:        classfoundation.ResolutionResolvedExisting,
		CanonicalClaimKey: string(claimKey),
		ProfileObservation: &classfoundation.ObservedProfileObservation{
			ClassTermID:       classTermID,
			AttributeKey:      "metric_value",
			ObservedName:      strings.TrimSpace(input.MetricName),
			LogicalDatatype:   "raw_metric_value",
			ValueForm:         "raw",
			UnitTermID:        strings.TrimSpace(input.MetricUnit),
			ObservationState:  observationState,
			AggregationMethod: "metric_foundation_shadow",
			MethodVersion:     MetricAdapterVersion,
			RawValue:          rawValue,
		},
		Redirect: redirect,
	}, nil
}
