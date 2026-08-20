package assertions

import "github.com/chendingplano/deepdoc/server/api/ontology/semantic"
import "testing"

// Task 7.2 (and task 7.6's follow-on): every family with a registered
// normalizer (seam 5) but no compliant semantic-instance adapter must be
// discoverable through the shared adapter registry via the generic
// fallback. As of task 7.6 both registered families -- "metric"
// (MetricAdapter) and "provision" (ProvisionAdapter) -- have graduated to a
// real, non-fallback adapter, so EnsureGenericFallbackAdapters wires nothing
// today; it left each family's own real adapter untouched rather than
// overwriting it. A future family added to the normalizer registry before
// it earns a real adapter (7.7's remaining families) is exactly the case
// this function exists for.
func TestEnsureGenericFallbackAdaptersWiresNothingWhenEveryFamilyHasARealAdapter(t *testing.T) {
	wired := EnsureGenericFallbackAdapters()
	if len(wired) != 0 {
		t.Fatalf("wired = %v, want none: every registered family already has a real adapter", wired)
	}

	metricAdapter, ok := semantic.LookupAdapter(semantic.MetricArtifactType)
	if !ok {
		t.Fatal("expected the real metric adapter to already be registered")
	}
	if metricAdapter.AdapterName() != semantic.MetricAdapterName {
		t.Errorf("metric's own compliant adapter must not be replaced by the generic fallback, got adapter name %q", metricAdapter.AdapterName())
	}
	if !metricAdapter.SupportsInstances() {
		t.Error("metric's real adapter must still claim instance support")
	}

	provisionAdapter, ok := semantic.LookupAdapter("provision")
	if !ok {
		t.Fatal("expected the real provision adapter to already be registered")
	}
	if provisionAdapter.AdapterName() != semantic.ProvisionAdapterName {
		t.Errorf("provision's own compliant adapter must not be replaced by the generic fallback, got adapter name %q", provisionAdapter.AdapterName())
	}
	if !provisionAdapter.SupportsInstances() {
		t.Error("provision's real adapter must claim instance support (task 7.6)")
	}
}

// Re-running the wiring (e.g. a second call in the same process) must not
// panic -- semantic.RegisterAdapter panics on double-registration, so this
// function must skip families it already wired.
func TestEnsureGenericFallbackAdaptersIsIdempotent(t *testing.T) {
	EnsureGenericFallbackAdapters()
	EnsureGenericFallbackAdapters()
}
