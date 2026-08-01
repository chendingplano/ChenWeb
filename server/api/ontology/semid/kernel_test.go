package semid

import (
	"context"
	"testing"
)

// ---- ADR kernel test 18: normalizer determinism and versioned re-index ----

func TestNormalizerDeterministicAcrossRuns(t *testing.T) {
	n := Normalizer{Name: "basic", Version: 1}
	a := n.Normalize("  Display   Module  ")
	b := n.Normalize("display module")
	if a.CanonicalKey != b.CanonicalKey || a.CanonicalKey != "display module" {
		t.Fatalf("expected deterministic key %q, got %q and %q", "display module", a.CanonicalKey, b.CanonicalKey)
	}
}

func TestNormalizerVersionBumpReindexesWithoutLosingSurface(t *testing.T) {
	n1 := Normalizer{Name: "basic", Version: 1}
	n2 := Normalizer{Name: "basic", Version: 2}
	// Punctuation-bearing surface: v1 keeps it, v2 strips it -> a re-index.
	if got := n1.Normalize("display, module").CanonicalKey; got != "display, module" {
		t.Fatalf("v1 key %q", got)
	}
	if got := n2.Normalize("display, module").CanonicalKey; got != "display module" {
		t.Fatalf("v2 key %q", got)
	}
	// Clean surface: keys are identical across versions (no data lost).
	if n1.Normalize("display module").CanonicalKey != n2.Normalize("display module").CanonicalKey {
		t.Fatal("expected clean surfaces to re-index identically")
	}
}

// ---- ADR kernel test 19: merge tombstone, stale-id resolution, unmerge ----

func TestMergeTombstoneResolveStaleAndUnmerge(t *testing.T) {
	g := NewMergeGraph()
	if err := g.Merge("A", "B"); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if to, ok := g.MergedInto("A"); !ok || to != "B" {
		t.Fatalf("expected A.merged_into=B, got %q ok=%v", to, ok)
	}
	if got := g.Resolve("A"); got != "B" {
		t.Fatalf("stale id A should resolve to B, got %q", got)
	}
	// The losing row is kept (tombstone), not deleted.
	if _, ok := g.MergedInto("A"); !ok {
		t.Fatal("tombstone A must be retained")
	}
	g.Unmerge("A")
	if got := g.Resolve("A"); got != "A" {
		t.Fatalf("after unmerge A should resolve to itself, got %q", got)
	}
}

// ---- ADR kernel test 20: no transitive closure over pairwise merges ----

func TestMergeNoTransitiveClosure(t *testing.T) {
	g := NewMergeGraph()
	if err := g.Merge("A", "B"); err != nil {
		t.Fatalf("Merge A->B: %v", err)
	}
	if err := g.Merge("B", "C"); err != nil {
		t.Fatalf("Merge B->C: %v", err)
	}
	// Resolution follows the chain, but no A->C decision is fabricated.
	if got := g.Resolve("A"); got != "C" {
		t.Fatalf("expected A to resolve to C via the chain, got %q", got)
	}
	if to, ok := g.MergedInto("A"); !ok || to != "B" {
		t.Fatalf("A's tombstone must stay B (no A->C decision row), got %q ok=%v", to, ok)
	}
	// Only two pairwise decisions exist.
	if len(g.mergedInto) != 2 {
		t.Fatalf("expected exactly 2 pairwise decisions, got %d", len(g.mergedInto))
	}
}

// ---- ADR kernel test 21: never_merge is never violated ----

func TestNeverMergeBlocksAutomaticMerge(t *testing.T) {
	g := NewMergeGraph()
	g.SetNeverMerge("A", "B")
	if !g.IsNeverMerge("A", "B") || !g.IsNeverMerge("B", "A") {
		t.Fatal("never_merge pair must be symmetric")
	}
	if err := g.Merge("A", "B"); err == nil {
		t.Fatal("expected Merge A->B to be blocked by never_merge")
	}
	if err := g.Merge("B", "A"); err == nil {
		t.Fatal("expected Merge B->A to be blocked by never_merge")
	}
}

// ---- kernel adjudication: governed families never auto-accept ----

type fakeFamily struct {
	nodes []NodeCandidate
}

func (f fakeFamily) FamilyName() string                       { return "fake" }
func (f fakeFamily) Normalizer() Normalizer                   { return Normalizer{Name: "basic", Version: 1} }
func (f fakeFamily) AutoAcceptPolicy() AutoAcceptPolicy       { return AutoAcceptPolicy{Enabled: false} }
func (f fakeFamily) Scope(string) string                      { return "" }
func (f fakeFamily) CandidateNodes(ctx context.Context, s, scope string) ([]NodeCandidate, error) {
	return f.nodes, nil
}

func TestKernelResolveGovernedFamilyEndsAtHumanReview(t *testing.T) {
	// Even a perfect candidate match must end at human review for a governed
	// family (terms): adjudication is a change set, never an auto-accept.
	fam := fakeFamily{nodes: []NodeCandidate{
		{NodeID: "core:assertion", KeyBundle: KeyBundle{CanonicalKey: "assertion"}},
	}}
	res, err := (Kernel{Family: fam}).Resolve(context.Background(), "Assertion")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Verdict != VerdictHumanReview {
		t.Fatalf("governed family must end at human_review, got %s", res.Verdict)
	}
	if len(res.Matches) != 1 || res.Matches[0].NodeID != "core:assertion" || res.Matches[0].Score != 1.0 {
		t.Fatalf("unexpected matches: %#v", res.Matches)
	}
}

func TestAdjudicateVerdicts(t *testing.T) {
	// No candidates -> deferred.
	if v := Adjudicate(nil, AutoAcceptPolicy{Enabled: true, MinScore: 0.9, MaxCandidates: 1}); v != VerdictDeferred {
		t.Fatalf("expected deferred, got %s", v)
	}
	// Auto-accept enabled + above min -> auto_accepted.
	matches := []ScoredMatch{{NodeID: "n", Score: 1.0}}
	if v := Adjudicate(matches, AutoAcceptPolicy{Enabled: true, MinScore: 0.9, MaxCandidates: 1}); v != VerdictAutoAccept {
		t.Fatalf("expected auto_accepted, got %s", v)
	}
	// Below min -> human_review.
	if v := Adjudicate([]ScoredMatch{{NodeID: "n", Score: 0.5}}, AutoAcceptPolicy{Enabled: true, MinScore: 0.9, MaxCandidates: 1}); v != VerdictHumanReview {
		t.Fatalf("expected human_review, got %s", v)
	}
	// Tied top candidates -> ambiguous.
	tied := []ScoredMatch{{NodeID: "a", Score: 1.0}, {NodeID: "b", Score: 1.0}}
	if v := Adjudicate(tied, AutoAcceptPolicy{Enabled: true, MinScore: 0.9, MaxCandidates: 1}); v != VerdictAmbiguous {
		t.Fatalf("expected ambiguous, got %s", v)
	}
}
