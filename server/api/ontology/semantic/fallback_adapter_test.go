package semantic

import "testing"

// Task 7.2: the generic fallback adapter must itself pass the same shared
// conformance suite a real family adapter does -- DR13 makes suite passage
// the precondition for a writer mode to be recorded at all, fallback
// included.
func TestFallbackAdapterPassesConformanceSuite(t *testing.T) {
	res := RunConformanceSuite(FallbackAdapter{ArtifactTypeValue: "provision"})
	if !res.Passed {
		t.Fatalf("FallbackAdapter failed conformance: %v", res.Failures)
	}
	if res.SuiteVersion != ConformanceSuiteVersion {
		t.Errorf("suite version = %q, want %q", res.SuiteVersion, ConformanceSuiteVersion)
	}
}

// DR1: only a family with a compliant instance adapter may claim instance
// support. The generic fallback must never claim it on a family's behalf --
// that would let AuthorizeWriterActivation treat a fallback-only family as
// eligible for the ontology-instance path it has no adapter for.
func TestFallbackAdapterDeclaresNoInstanceSupport(t *testing.T) {
	if (FallbackAdapter{ArtifactTypeValue: "provision"}).SupportsInstances() {
		t.Fatal("a fallback adapter must never claim instance support (DR1)")
	}
}

// Two families sharing this one Go implementation must still get distinct
// occurrence scopes, or OccurrenceKey could collide across families.
func TestFallbackAdapterScopesPerArtifactType(t *testing.T) {
	a := FallbackAdapter{ArtifactTypeValue: "provision"}
	b := FallbackAdapter{ArtifactTypeValue: "entity"}
	if a.OccurrenceScope() == b.OccurrenceScope() {
		t.Fatalf("two families must not share an occurrence scope, both got %q", a.OccurrenceScope())
	}
}
