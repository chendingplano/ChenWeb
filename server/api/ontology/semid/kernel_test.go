package semid

import (
	"context"
	"testing"
)

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

// ---- kernel ----

type fakeFamily struct {
	nodes     []NodeCandidate
	policy    AutoAcceptPolicy
	inputSeen string
	scopeSeen string
}

func (f *fakeFamily) FamilyName() string                 { return "fake" }
func (f *fakeFamily) AutoAcceptPolicy() AutoAcceptPolicy { return f.policy }
func (f *fakeFamily) CandidateNodes(ctx context.Context, input, scope string) ([]NodeCandidate, error) {
	f.inputSeen = input
	f.scopeSeen = scope
	return f.nodes, nil
}

// Governed families never auto-accept: even a perfect match ends at
// human_review. Also asserts K2: the caller's scope reaches both the
// candidate search and the resolution.
func TestKernelResolveGovernedFamilyEndsAtHumanReview(t *testing.T) {
	fam := &fakeFamily{
		nodes: []NodeCandidate{
			{NodeID: "core:assertion", KeyBundle: KeyBundle{CanonicalKey: "assertion"}, Method: "tier1_norm"},
		},
		policy: AutoAcceptPolicy{Enabled: false, MaxCandidates: 1},
	}
	res, err := (Kernel{Family: fam, Normalizer: Normalizer{Version: CurrentNormalizerVersion}}).Resolve(context.Background(), "Assertion", "mea")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Verdict != VerdictHumanReview {
		t.Fatalf("governed family must end at human_review, got %s", res.Verdict)
	}
	if res.Scope != "mea" || fam.scopeSeen != "mea" {
		t.Errorf("scope must round-trip into the search and the resolution (K2): res.Scope=%q, search scope=%q", res.Scope, fam.scopeSeen)
	}
	if res.Input != "Assertion" {
		t.Errorf("Input: got %q, want %q", res.Input, "Assertion")
	}
	if len(res.Matches) != 1 || res.Matches[0].NodeID != "core:assertion" || res.Matches[0].Score != 1.0 {
		t.Fatalf("unexpected matches: %#v", res.Matches)
	}
	// §8.3: human_review carries the top-1 id, flagged for sampling.
	if res.ResolvedNodeID != "core:assertion" || res.Method != "tier1_norm" {
		t.Errorf("human_review must carry the top-1 id and method (§8.3): got %q/%q", res.ResolvedNodeID, res.Method)
	}
}

// §8.3/D11: ambiguous carries the top-1 plus the tied set.
func TestKernelAmbiguousCarriesTop1(t *testing.T) {
	fam := &fakeFamily{
		nodes: []NodeCandidate{
			{NodeID: "kwc_b", KeyBundle: KeyBundle{CanonicalKey: "assertion"}, Method: "tier1_norm"},
			{NodeID: "kwc_a", KeyBundle: KeyBundle{CanonicalKey: "assertion"}, Method: "tier1_norm"},
		},
		policy: AutoAcceptPolicy{Enabled: true, MinScore: 0.8, MaxCandidates: 1},
	}
	res, err := (Kernel{Family: fam, Normalizer: Normalizer{Version: CurrentNormalizerVersion}}).Resolve(context.Background(), "assertion", "_")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Verdict != VerdictAmbiguous {
		t.Fatalf("tied top candidates must be ambiguous, got %s", res.Verdict)
	}
	if res.ResolvedNodeID != "kwc_a" {
		t.Errorf("ambiguous must carry the deterministic top-1, got %q", res.ResolvedNodeID)
	}
	if res.Method != "tier1_norm" {
		t.Errorf("ambiguous must carry the top-1 method, got %q", res.Method)
	}
	if len(res.Matches) != 2 {
		t.Errorf("the tied set must be preserved, got %d matches", len(res.Matches))
	}
}

func TestKernelNoCandidatesDefers(t *testing.T) {
	fam := &fakeFamily{policy: AutoAcceptPolicy{Enabled: true, MinScore: 0.8, MaxCandidates: 1}}
	res, err := (Kernel{Family: fam, Normalizer: Normalizer{Version: CurrentNormalizerVersion}}).Resolve(context.Background(), "anything", "_")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Verdict != VerdictDeferred {
		t.Fatalf("no candidates must defer, got %s", res.Verdict)
	}
	if res.ResolvedNodeID != "" {
		t.Errorf("deferred carries no id on the collector path, got %q", res.ResolvedNodeID)
	}
}

// Score is symmetric over the two bundles: the tier-2/tier-4 bridges need a
// query key to match a stored alternate, and vice versa.
func TestScoreSymmetricAlternateMatch(t *testing.T) {
	// Tier 4 shape: the query's canonical key hits a stored initials key.
	query := KeyBundle{CanonicalKey: "vdm"}
	cand := KeyBundle{CanonicalKey: "visual display module", AlternateKeys: []string{"visualdisplaymodule", "vdm"}}
	if s := Score(query, cand); s != 0.8 {
		t.Errorf("initials bridge: got %v, want 0.8", s)
	}
	// Tier 2 shape: a shared alternate key (e.g. alnum) on both sides.
	q2 := KeyBundle{CanonicalKey: "c++ tools", AlternateKeys: []string{"ctools"}}
	c2 := KeyBundle{CanonicalKey: "c tools", AlternateKeys: []string{"ctools"}}
	if s := Score(q2, c2); s != 0.8 {
		t.Errorf("shared alternate: got %v, want 0.8", s)
	}
	// Candidate canonical inside the query's alternates (sorted bridge).
	q3 := KeyBundle{CanonicalKey: "module display", AlternateKeys: []string{"display module"}}
	c3 := KeyBundle{CanonicalKey: "display module"}
	if s := Score(q3, c3); s != 0.8 {
		t.Errorf("sorted bridge: got %v, want 0.8", s)
	}
	// Exact canonical match.
	if s := Score(KeyBundle{CanonicalKey: "x"}, KeyBundle{CanonicalKey: "x"}); s != 1.0 {
		t.Errorf("exact: got %v, want 1.0", s)
	}
	// No shared key, no prefix.
	if s := Score(KeyBundle{CanonicalKey: "alpha"}, KeyBundle{CanonicalKey: "beta"}); s != 0 {
		t.Errorf("disjoint: got %v, want 0", s)
	}
	// Mutual prefix >= 3 chars.
	if s := Score(KeyBundle{CanonicalKey: "cloud"}, KeyBundle{CanonicalKey: "cloud computing"}); s != 0.5 {
		t.Errorf("prefix: got %v, want 0.5", s)
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
	// Tied top candidates beyond MaxCandidates -> ambiguous.
	tied := []ScoredMatch{{NodeID: "a", Score: 1.0}, {NodeID: "b", Score: 1.0}}
	if v := Adjudicate(tied, AutoAcceptPolicy{Enabled: true, MinScore: 0.9, MaxCandidates: 1}); v != VerdictAmbiguous {
		t.Fatalf("expected ambiguous, got %s", v)
	}
	// Governed family with explicit MaxCandidates (K10): ties are detectable
	// again — ambiguous before the enabled/min-score gate.
	if v := Adjudicate(tied, AutoAcceptPolicy{Enabled: false, MaxCandidates: 1}); v != VerdictAmbiguous {
		t.Fatalf("expected ambiguous for governed tie with explicit MaxCandidates, got %s", v)
	}
}
