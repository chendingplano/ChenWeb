package keywords

import (
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/semid"
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

// D3: the family carries no normalizer of its own — it runs the one shared
// semid pipeline, defaulting to the current normalizer version.
func TestKeywordFamilyDefaultsToCurrentNormalizerVersion(t *testing.T) {
	kf := &KeywordFamily{}
	kf.ensureDefaults()
	if kf.NormalizerVersion != semid.CurrentNormalizerVersion {
		t.Errorf("NormalizerVersion: got %d, want %d", kf.NormalizerVersion, semid.CurrentNormalizerVersion)
	}
	if kf.normalizer().Version != semid.CurrentNormalizerVersion {
		t.Errorf("normalizer().Version: got %d, want %d", kf.normalizer().Version, semid.CurrentNormalizerVersion)
	}
}

func TestKeywordFamilyResolveSurfaceOff(t *testing.T) {
	kf := &KeywordFamily{ResolverMode: "off"}
	res, err := kf.ResolveSurface(nil, "test", "_")
	if err != nil {
		t.Fatalf("ResolveSurface (off): %v", err)
	}
	if res.Verdict != "" {
		t.Errorf("expected zero resolution in off mode, got verdict %q", res.Verdict)
	}
}

func TestKeywordFamilyResolveSurfaceNoDB(t *testing.T) {
	// No DB + observe mode should still no-op gracefully.
	kf := &KeywordFamily{ResolverMode: "observe"}
	res, err := kf.ResolveSurface(nil, "test", "_")
	if err != nil {
		t.Fatalf("ResolveSurface (observe, no DB): %v", err)
	}
	if res.Verdict != "" {
		t.Errorf("expected zero resolution when DB is nil, got verdict %q", res.Verdict)
	}
}

func TestKeywordFamilyObserveSurfaceOff(t *testing.T) {
	kf := &KeywordFamily{ResolverMode: "off"}
	res, err := kf.ObserveSurface(nil, "test", "_", "art", "ctx")
	if err != nil {
		t.Fatalf("ObserveSurface (off): %v", err)
	}
	if res != nil {
		t.Error("expected nil resolution in off mode")
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
