package assertions

import "github.com/chendingplano/deepdoc/server/api/ontology/semantic"
import "testing"

// Task 7.2: every family with a registered normalizer (seam 5) but no
// compliant semantic-instance adapter must be discoverable through the
// shared adapter registry via the generic fallback. "metric" already has a
// real adapter (MetricAdapter, registered by its own init()) and must be
// left untouched; "provision" has none yet and must get a FallbackAdapter.
func TestEnsureGenericFallbackAdaptersWiresProvisionNotMetric(t *testing.T) {
	EnsureGenericFallbackAdapters()

	provisionAdapter, ok := semantic.LookupAdapter("provision")
	if !ok {
		t.Fatal("expected a fallback adapter registered for \"provision\"")
	}
	if provisionAdapter.SupportsInstances() {
		t.Error("the generic fallback adapter must never claim instance support")
	}

	metricAdapter, ok := semantic.LookupAdapter(semantic.MetricArtifactType)
	if !ok {
		t.Fatal("expected the real metric adapter to already be registered")
	}
	if metricAdapter.AdapterName() != semantic.MetricAdapterName {
		t.Errorf("metric's own compliant adapter must not be replaced by the generic fallback, got adapter name %q", metricAdapter.AdapterName())
	}
}

// Re-running the wiring (e.g. a second call in the same process) must not
// panic -- semantic.RegisterAdapter panics on double-registration, so this
// function must skip families it already wired.
func TestEnsureGenericFallbackAdaptersIsIdempotent(t *testing.T) {
	EnsureGenericFallbackAdapters()
	EnsureGenericFallbackAdapters()
}
