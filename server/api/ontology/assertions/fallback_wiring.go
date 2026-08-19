package assertions

import (
	"sort"

	"github.com/chendingplano/deepdoc/server/api/ontology/semantic"
)

// EnsureGenericFallbackAdapters registers a semantic.FallbackAdapter (DR13's
// generic option-3 declaration) for every family with a registered
// normalizer (seam 5) that has no compliant semantic-instance adapter of its
// own yet. A family that already has one (metric, via MetricAdapter) is left
// untouched.
//
// It only registers -- it never runs the conformance suite and never touches
// the database. Callers that want a compliance record (the
// fallback-conformance command) call semantic.VerifyAndRecord themselves for
// each artifact type this function reports.
//
// Safe to call more than once in the same process: a family already wired
// (by a prior call, or because it since earned a real adapter) is skipped
// rather than left to semantic.RegisterAdapter's duplicate-registration
// panic.
func EnsureGenericFallbackAdapters() []string {
	var wired []string
	for _, family := range RegisteredFamilies() {
		if _, ok := semantic.LookupAdapter(family); ok {
			continue
		}
		semantic.RegisterAdapter(semantic.FallbackAdapter{ArtifactTypeValue: family})
		wired = append(wired, family)
	}
	sort.Strings(wired)
	return wired
}
