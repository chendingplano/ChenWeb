package semid

import (
	"testing"
)

// §18.2 required set: a normalizer table containing AIDS/SaaS/Kubernetes/
// AWS's. N1: singularization must never destroy acronyms and proper nouns —
// not in Norm (singularization is an alternate key, never part of Norm) and
// not as a Singular alternate (the casing signal skips tokens whose original
// form carried uppercase).
func TestNormalizerN1Table(t *testing.T) {
	n := Normalizer{Version: CurrentNormalizerVersion}
	tests := []struct {
		surface    string
		wantNorm   string
		wantSing   string
		bannedAlts []string
	}{
		{"AIDS", "aids", "aids", []string{"aid"}},
		{"SaaS", "saas", "saas", []string{"saa"}},
		{"Kubernetes", "kubernetes", "kubernetes", []string{"kubernete"}},
		{"Postgres", "postgres", "postgres", []string{"postgre"}},
		{"analysis", "analysis", "analysis", []string{"analysi"}},
		{"bias", "bias", "bias", []string{"bia"}},
		{"status", "status", "status", []string{"statu"}},
	}
	for _, tt := range tests {
		ks := n.Normalize(tt.surface)
		if ks.Norm != tt.wantNorm {
			t.Errorf("Normalize(%q).Norm = %q, want %q", tt.surface, ks.Norm, tt.wantNorm)
		}
		if ks.Singular != tt.wantSing {
			t.Errorf("Normalize(%q).Singular = %q, want %q", tt.surface, ks.Singular, tt.wantSing)
		}
		for _, banned := range tt.bannedAlts {
			for _, a := range ks.Bundle().AlternateKeys {
				if a == banned {
					t.Errorf("Normalize(%q) alternates contain banned %q: %v", tt.surface, banned, ks.Bundle().AlternateKeys)
				}
			}
		}
	}
}

// N2: possessive stripping must work at the word-final position too —
// AWS's → aws, not aws'.
func TestNormalizerN2Possessives(t *testing.T) {
	n := Normalizer{Version: CurrentNormalizerVersion}
	tests := []struct{ surface, want string }{
		{"AWS's", "aws"},
		{"patient's monitor", "patient monitor"},
		{"the patient's monitor", "patient monitor"},
	}
	for _, tt := range tests {
		if got := n.Normalize(tt.surface).Norm; got != tt.want {
			t.Errorf("Normalize(%q).Norm = %q, want %q", tt.surface, got, tt.want)
		}
	}
}

// Singularization lives in the Singular alternate key: exception-aware,
// stop-listed, and guarded.
func TestNormalizerSingularAlternates(t *testing.T) {
	n := Normalizer{Version: CurrentNormalizerVersion}
	tests := []struct{ surface, want string }{
		{"indices", "index"},
		{"criteria", "criterion"},
		{"analyses", "analysis"},
		{"children", "child"},
		{"categories", "category"},
		{"classes", "class"},
		{"watches", "watch"},
		{"boxes", "box"},
		{"data analyses", "data analysis"},
		// Lowercase plurals of proper-noun-headed phrases still singularize
		// the lowercase tokens only.
		{"Kubernetes clusters", "kubernetes cluster"},
	}
	for _, tt := range tests {
		if got := n.Normalize(tt.surface).Singular; got != tt.want {
			t.Errorf("Normalize(%q).Singular = %q, want %q", tt.surface, got, tt.want)
		}
	}
}

// N3: every key is lower-cased; initials included.
func TestNormalizerInitialsLowercase(t *testing.T) {
	n := Normalizer{Version: CurrentNormalizerVersion}
	if got := n.Normalize("Visual Display Module").Initials; got != "vdm" {
		t.Errorf("Initials: got %q, want %q", got, "vdm")
	}
	if got := n.Normalize("AIDS").Initials; got != "a" {
		t.Errorf("Initials: got %q, want %q", got, "a")
	}
}

// §6.3 item 1: full Unicode case folding, not ToLower.
func TestNormalizerFullCaseFolding(t *testing.T) {
	n := Normalizer{Version: CurrentNormalizerVersion}
	// Sharp s folds to "ss" under full case folding; ToLower would keep ß.
	if got := n.Normalize("Straße").Norm; got != "strasse" {
		t.Errorf("Normalize(Straße).Norm = %q, want %q", got, "strasse")
	}
}

// §6.3 item 2 / §7 item 2: English-profile steps stay off CJK strings.
func TestNormalizerCJKPassthrough(t *testing.T) {
	n := Normalizer{Version: CurrentNormalizerVersion}
	ks := n.Normalize("数据库 索引")
	if ks.Norm != "数据库 索引" {
		t.Errorf("CJK Norm: got %q", ks.Norm)
	}
	if ks.Alnum != "数据库索引" {
		t.Errorf("CJK Alnum must keep Han characters, got %q", ks.Alnum)
	}
	// F7: singularization is an English-only operation and must not run at
	// all on a non-Latin string — not run-and-happen-to-be-a-no-op, which is
	// what an unguarded call produces for suffix patterns that never match
	// CJK text but would wrongly fire on any Latin fragment mixed into an
	// otherwise-CJK string (e.g. "显示屏幕亮度 nits" → "nit").
	if ks.Singular != "" {
		t.Errorf("CJK Singular must be empty (the English singularizer must not run), got %q", ks.Singular)
	}
}

// F7: a mixed CJK/Latin string is classified non-Latin by latinProfile (Han
// count >= Latin count), so the Latin fragment must not be singularized
// either — the bug this regression test targets ran singularKey
// unconditionally, singularizing "nits" to "nit" inside a majority-CJK
// string despite the Latin gate already protecting possessives/articles.
func TestNormalizerSingularNeverRunsOnNonLatinProfile(t *testing.T) {
	n := Normalizer{Version: CurrentNormalizerVersion}
	if latinProfile("显示屏幕亮度 nits") {
		t.Fatal("test premise violated: expected latinProfile to be false for this string")
	}
	if got := n.Normalize("显示屏幕亮度 nits").Singular; got != "" {
		t.Errorf("Singular must be empty when latinProfile is false, got %q", got)
	}
}

// The punctuation pipeline: zero-width chars, dashes, quotes, whitespace,
// dotted initialisms, leading articles.
func TestNormalizerPunctuationPipeline(t *testing.T) {
	n := Normalizer{Version: CurrentNormalizerVersion}
	tests := []struct{ surface, want string }{
		{"display\u200Bmodule", "displaymodule"},
		{"display—module", "display-module"},
		{"‘quoted’", "'quoted'"},
		{"  Display   Module  ", "display module"},
		{"U.S.A.", "usa"},
		{"The Cloud", "cloud"},
		{"an apple", "apple"},
	}
	for _, tt := range tests {
		if got := n.Normalize(tt.surface).Norm; got != tt.want {
			t.Errorf("Normalize(%q).Norm = %q, want %q", tt.surface, got, tt.want)
		}
	}
}

// Determinism: same surface, same version, same keys — across whitespace
// variants too.
func TestNormalizerDeterministic(t *testing.T) {
	n := Normalizer{Version: CurrentNormalizerVersion}
	a := n.Normalize("  Display   Module  ")
	b := n.Normalize("display module")
	if a.Norm != b.Norm || a.Norm != "display module" {
		t.Fatalf("expected deterministic key %q, got %q and %q", "display module", a.Norm, b.Norm)
	}
	if a.Bundle().CanonicalKey != b.Bundle().CanonicalKey {
		t.Fatal("bundles must agree on the canonical key")
	}
}

// Bundle: canonical is Norm; alternates are deduplicated and never repeat
// the canonical key.
func TestKeySetBundle(t *testing.T) {
	n := Normalizer{Version: CurrentNormalizerVersion}

	ks := n.Normalize("analyses")
	b := ks.Bundle()
	if b.CanonicalKey != "analyses" {
		t.Errorf("CanonicalKey: got %q, want %q", b.CanonicalKey, "analyses")
	}
	found := false
	for _, a := range b.AlternateKeys {
		if a == "analysis" {
			found = true
		}
		if a == b.CanonicalKey {
			t.Errorf("alternates must not repeat the canonical key: %v", b.AlternateKeys)
		}
	}
	if !found {
		t.Errorf("alternates must carry the singular bridge %q: %v", "analysis", b.AlternateKeys)
	}

	// Singular equal to Norm carries no signal and is omitted.
	for _, a := range n.Normalize("analysis").Bundle().AlternateKeys {
		if a == "analysis" {
			t.Errorf("Singular==Norm must be omitted from alternates, got %v", a)
		}
	}
}
