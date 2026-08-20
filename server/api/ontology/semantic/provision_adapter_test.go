package semantic

import "testing"

// The real provision adapter must pass its own suite; otherwise the writer
// gate could never be authorized once activation is decided.
func TestProvisionAdapterPassesConformanceSuite(t *testing.T) {
	res := RunConformanceSuite(ProvisionAdapter{})
	if !res.Passed {
		t.Fatalf("ProvisionAdapter failed conformance: %v", res.Failures)
	}
}

// DR1: task 7.6 gives provisions a real writer, so unlike the generic
// fallback declaration, the real adapter does claim instance support.
func TestProvisionAdapterDeclaresInstanceSupport(t *testing.T) {
	if !(ProvisionAdapter{}).SupportsInstances() {
		t.Fatal("provisions are modelled as ontology object instances as of task 7.6")
	}
}

func TestProvisionAdapterDeclaresOneRequiredStage(t *testing.T) {
	stages := ProvisionAdapter{}.RequiredStages()
	if len(stages) != 1 {
		t.Fatalf("required stage count = %d, want 1 (associate only -- no separate normalize/class-resolution phase)", len(stages))
	}
	if stages[0].StageTermID != StageAssociate {
		t.Errorf("required stage = %q, want %q", stages[0].StageTermID, StageAssociate)
	}
}
