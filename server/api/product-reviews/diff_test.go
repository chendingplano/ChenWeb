package productreviews

import "testing"

func res(id, tier string) Result {
	return Result{ArtifactType: "metric", ArtifactID: id, Tier: tier}
}

// Scenario: New documents surface as added artifacts + tier changes are reported
// (spec: Re-run and diff).
func TestDiffAddedRemovedRetiered(t *testing.T) {
	prev := []Result{res("a", TierPart), res("b", TierDocumentScope), res("c", TierAspect)}
	cur := []Result{res("a", TierDirect) /* re-tiered */, res("c", TierAspect) /* same */, res("d", TierPart) /* added */}
	// b removed.
	d := DiffResults(prev, cur, 3, 3, 1)

	if len(d.Added) != 1 || d.Added[0].ArtifactID != "d" {
		t.Fatalf("added = %+v, want [d]", d.Added)
	}
	if len(d.Removed) != 1 || d.Removed[0].ArtifactID != "b" {
		t.Fatalf("removed = %+v, want [b]", d.Removed)
	}
	if len(d.Retiered) != 1 || d.Retiered[0].ArtifactID != "a" ||
		d.Retiered[0].From != TierPart || d.Retiered[0].To != TierDirect {
		t.Fatalf("retiered = %+v, want a: part→direct", d.Retiered)
	}
	if d.ProfileVersionChanged {
		t.Fatal("profile version did not change; flag must be false")
	}
}

// Scenario: Diff flags a profile change (spec).
func TestDiffProfileVersionFlag(t *testing.T) {
	d := DiffResults([]Result{res("a", TierPart)}, []Result{res("a", TierPart)}, 3, 4, 2)
	if !d.ProfileVersionChanged || d.PreviousProfileVersion != 3 || d.CurrentProfileVersion != 4 {
		t.Fatalf("version flag wrong: %+v", d)
	}
}

// Scenario: Stable corpus yields an empty diff (spec).
func TestDiffStableEmpty(t *testing.T) {
	same := []Result{res("a", TierPart), res("b", TierDocumentScope)}
	d := DiffResults(same, same, 5, 5, 4)
	if len(d.Added) != 0 || len(d.Removed) != 0 || len(d.Retiered) != 0 {
		t.Fatalf("expected empty diff, got %+v", d)
	}
}
