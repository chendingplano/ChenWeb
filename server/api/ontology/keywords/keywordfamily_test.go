package keywords

import (
	"testing"
)

func TestKeywordFamilyName(t *testing.T) {
	kf := &KeywordFamily{}
	if kf.FamilyName() != "keyword" {
		t.Errorf("FamilyName: got %q, want %q", kf.FamilyName(), "keyword")
	}
}

func TestKeywordFamilyAutoAcceptPolicy(t *testing.T) {
	kf := &KeywordFamily{}
	p := kf.AutoAcceptPolicy()
	if !p.Enabled {
		t.Error("auto-accept should be enabled for keywords")
	}
	if p.MinScore != 0.8 {
		t.Errorf("MinScore: got %v, want 0.8", p.MinScore)
	}
	if p.MaxCandidates != 1 {
		t.Errorf("MaxCandidates: got %v, want 1", p.MaxCandidates)
	}
}

func TestKeywordFamilyNormalizer(t *testing.T) {
	kf := &KeywordFamily{NormalizerVersion: 1}
	n := kf.Normalizer()
	if n.Name != "keyword" {
		t.Errorf("Name: got %q, want %q", n.Name, "keyword")
	}
	if n.Version != 1 {
		t.Errorf("Version: got %d, want 1", n.Version)
	}
	if n.NormFunc == nil {
		t.Fatal("NormFunc should not be nil")
	}
	kb := n.Normalize("Hello World")
	if kb.CanonicalKey != "hello world" {
		t.Errorf("CanonicalKey: got %q, want %q", kb.CanonicalKey, "hello world")
	}
	if len(kb.AlternateKeys) < 3 {
		t.Errorf("expected at least 3 alternate keys, got %d", len(kb.AlternateKeys))
	}
}

func TestKeywordFamilyScope(t *testing.T) {
	kf := &KeywordFamily{}
	if kf.Scope("anything") != "_" {
		t.Errorf("Scope: got %q, want %q", kf.Scope("anything"), "_")
	}
}

func TestKeywordFamilyResolveSurfaceOff(t *testing.T) {
	kf := &KeywordFamily{ResolverMode: "off"}
	res, err := kf.ResolveSurface(nil, "test", "_", "", "")
	if err != nil {
		t.Fatalf("ResolveSurface (off): %v", err)
	}
	if res != nil {
		t.Error("expected nil resolution in off mode")
	}
}

func TestKeywordFamilyResolveSurfaceNoDB(t *testing.T) {
	// No DB + observe mode should still no-op gracefully.
	kf := &KeywordFamily{ResolverMode: "observe"}
	res, err := kf.ResolveSurface(nil, "test", "_", "", "")
	if err != nil {
		t.Fatalf("ResolveSurface (observe, no DB): %v", err)
	}
	if res != nil {
		t.Error("expected nil resolution when DB is nil")
	}
}

func TestKeywordFamilyCandidateNodesOff(t *testing.T) {
	kf := &KeywordFamily{ResolverMode: "off"}
	candidates, err := kf.CandidateNodes(nil, "test", "_")
	if err != nil {
		t.Fatalf("CandidateNodes (off): %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates in off mode, got %d", len(candidates))
	}
}

func TestKeywordFamilyCandidateNodesNoDB(t *testing.T) {
	kf := &KeywordFamily{ResolverMode: "observe"}
	candidates, err := kf.CandidateNodes(nil, "test", "_")
	if err != nil {
		t.Fatalf("CandidateNodes (observe, no DB): %v", err)
	}
	if len(candidates) != 0 {
		t.Errorf("expected 0 candidates with nil DB, got %d", len(candidates))
	}
}

func TestKeywordNormalizerToSemidKeyBundleMapping(t *testing.T) {
	// Verify the mapping produces valid semid KeyBundle for tiers.
	n := KeywordNormalizer{Version: 1}
	kb := n.Normalize("Hello World")

	canonical, alternates := kb.ToSemidKeyBundle()
	if canonical != "hello world" {
		t.Errorf("canonical: got %q", canonical)
	}

	// alternates should contain alnum, sorted, phonetic, initials.
	found := make(map[string]bool)
	for _, a := range alternates {
		found[a] = true
	}
	if !found[kb.Initials] {
		t.Errorf("initials %q not found in alternates: %v", kb.Initials, alternates)
	}
}
