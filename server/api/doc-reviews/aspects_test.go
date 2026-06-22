package docreviews

import (
	"testing"
)

func TestListAspects_CountAndBasicFields(t *testing.T) {
	aspects := ListAspects()

	// Count: the spec defines 46 aspects across 6 passes.
	if len(aspects) < 40 {
		t.Errorf("ListAspects returned %d entries, want at least 40", len(aspects))
	}

	for _, a := range aspects {
		if a.Name == "" {
			t.Error("Found an aspect with empty Name")
		}
		if a.Group == "" {
			t.Errorf("Aspect %q has empty Group", a.Name)
		}
		if a.Label == "" {
			t.Errorf("Aspect %q has empty Label", a.Name)
		}
		// Groups should be P1-P6.
		if a.Group != "P1" && a.Group != "P2" && a.Group != "P3" &&
			a.Group != "P4" && a.Group != "P5" && a.Group != "P6" {
			t.Errorf("Aspect %q has unexpected Group %q", a.Name, a.Group)
		}
	}
}

func TestListAspects_AllGroupsRepresented(t *testing.T) {
	aspects := ListAspects()
	groups := map[string]int{}
	for _, a := range aspects {
		groups[a.Group]++
	}

	// All 6 pass groups should be present.
	for _, p := range []string{"P1", "P2", "P3", "P4", "P5", "P6"} {
		if groups[p] == 0 {
			t.Errorf("Group %s has 0 aspects", p)
		}
	}

	// P3 (Content Quality) and P5 (Technical & Compliance) are the largest groups.
	if groups["P3"] < 8 {
		t.Errorf("P3 has %d aspects, want at least 8", groups["P3"])
	}
	if groups["P5"] < 9 {
		t.Errorf("P5 has %d aspects, want at least 9", groups["P5"])
	}
}

func TestListTiers_CountAndContent(t *testing.T) {
	tiers := ListTiers()

	if len(tiers) != 4 {
		t.Fatalf("ListTiers returned %d tiers, want 4", len(tiers))
	}

	// Verify each tier has a non-empty aspect_names.
	for _, ti := range tiers {
		if len(ti.AspectNames) == 0 {
			t.Errorf("Tier %q has empty AspectNames", ti.Key)
		}
	}

	// Tier keys.
	expectedKeys := map[string]string{
		"must_review":     "Must Review",
		"should_review":   "Should Review",
		"review_external": "Review for External/Public",
		"review_regulated": "Review for Regulated",
	}
	for _, ti := range tiers {
		wantLabel, ok := expectedKeys[ti.Key]
		if !ok {
			t.Errorf("Unexpected tier key %q", ti.Key)
			continue
		}
		if ti.Label != wantLabel {
			t.Errorf("Tier %q label = %q, want %q", ti.Key, ti.Label, wantLabel)
		}
	}
}

func TestResolveAspectsForTier(t *testing.T) {
	// "must_review" should return aspects with Priority "Must Review".
	mustAspects := ResolveAspectsForTier("must_review")
	if len(mustAspects) == 0 {
		t.Fatal("ResolveAspectsForTier('must_review') returned empty slice")
	}

	// Verify every returned aspect is actually a must-review aspect.
	allAspects := ListAspects()
	mustSet := map[string]bool{}
	for _, a := range allAspects {
		if a.Priority == "Must Review" {
			mustSet[a.Name] = true
		}
	}
	for _, name := range mustAspects {
		if !mustSet[name] {
			t.Errorf("Aspect %q is in must_review tier but has Priority %q (expected 'Must Review')",
				name, findAspectPriority(allAspects, name))
		}
	}

	// Verify every "Must Review" aspect is included.
	for name := range mustSet {
		found := false
		for _, n := range mustAspects {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Aspect %q has 'Must Review' priority but is not in must_review tier", name)
		}
	}
}

func TestResolveAspectsForTier_Invalid(t *testing.T) {
	aspects := ResolveAspectsForTier("invalid_tier")
	if aspects != nil {
		t.Errorf("ResolveAspectsForTier('invalid_tier') = %v, want nil", aspects)
	}

	aspects = ResolveAspectsForTier("")
	if aspects != nil {
		t.Errorf("ResolveAspectsForTier('') = %v, want nil", aspects)
	}
}

func findAspectPriority(aspects []AspectInfo, name string) string {
	for _, a := range aspects {
		if a.Name == name {
			return a.Priority
		}
	}
	return "unknown"
}
