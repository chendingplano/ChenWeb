package semantic

import (
	"sort"

	"github.com/chendingplano/deepdoc/server/api/ontology/classfoundation"
)

// FoundationShadowException identifies an occurrence requiring follow-up
// without treating it as a writer failure.
type FoundationShadowException struct {
	MetricID      string
	InputRecordID int64
	Reason        string
}

type FoundationShadowRedirect struct {
	MetricID      string
	InputRecordID int64
	Resolution    classfoundation.RedirectResolution
}

// ClaimShadowConvergence groups source occurrences that compute the same
// canonical key. ClaimCollisions is intentionally separate: this read-only
// projection cannot manufacture a collision; registry lookups populate that
// category once a stored payload disagrees with a canonical key.
type ClaimShadowConvergence struct {
	CanonicalClaimKey string
	Occurrences       int
}

// FoundationShadowReport is bounded detailed evidence behind ShadowComparison
// counts. It does not persist diagnostics or invoke foundation stores.
type FoundationShadowReport struct {
	Limit     int
	Truncated bool

	ClassExceptions             []FoundationShadowException
	ClaimConvergences           []ClaimShadowConvergence
	ClaimCollisions             []FoundationShadowException
	ProfileOutliers             []FoundationShadowException
	Redirects                   []FoundationShadowRedirect
	DuplicateEvidenceExceptions []FoundationShadowException

	claimOccurrences map[string]int
}

func NewFoundationShadowReport(limit int) *FoundationShadowReport {
	if limit < 1 {
		limit = classfoundation.CapsFromEnv().ClaimShadowReportRows
	}
	return &FoundationShadowReport{Limit: limit, claimOccurrences: map[string]int{}}
}

func (r *FoundationShadowReport) Observe(metricID string, inputRecordID int64, result MetricFoundationShadowResult, supportLinks int) {
	if r == nil {
		return
	}
	if result.ClassState == MetricFoundationClassUnavailable {
		r.addException(&r.ClassExceptions, FoundationShadowException{MetricID: metricID, InputRecordID: inputRecordID, Reason: "source_class_unavailable"})
	}
	if result.CanonicalClaimKey != "" {
		r.claimOccurrences[result.CanonicalClaimKey]++
	}
	if result.ProfileObservation != nil && result.ProfileObservation.ObservationState != "represented" {
		r.addException(&r.ProfileOutliers, FoundationShadowException{MetricID: metricID, InputRecordID: inputRecordID, Reason: result.ProfileObservation.ObservationState})
	}
	if result.Redirect.Status != "" && (result.Redirect.Status != classfoundation.RedirectResolved || len(result.Redirect.Traversal) > 1) {
		if len(r.Redirects) >= r.Limit {
			r.Truncated = true
		} else {
			r.Redirects = append(r.Redirects, FoundationShadowRedirect{MetricID: metricID, InputRecordID: inputRecordID, Resolution: result.Redirect})
		}
	}
	if supportLinks > 1 {
		r.addException(&r.DuplicateEvidenceExceptions, FoundationShadowException{MetricID: metricID, InputRecordID: inputRecordID, Reason: "duplicate_current_metric_support"})
	}
}

func (r *FoundationShadowReport) Finalize() {
	if r == nil {
		return
	}
	keys := make([]string, 0, len(r.claimOccurrences))
	for key, occurrences := range r.claimOccurrences {
		if occurrences > 1 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		if len(r.ClaimConvergences) >= r.Limit {
			r.Truncated = true
			break
		}
		r.ClaimConvergences = append(r.ClaimConvergences, ClaimShadowConvergence{CanonicalClaimKey: key, Occurrences: r.claimOccurrences[key]})
	}
}

func (r *FoundationShadowReport) addException(target *[]FoundationShadowException, exception FoundationShadowException) {
	if len(*target) >= r.Limit {
		r.Truncated = true
		return
	}
	*target = append(*target, exception)
}
