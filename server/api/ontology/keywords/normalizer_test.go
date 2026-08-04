package keywords

import (
	"testing"
)

func TestNormalizerBasic(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	kb := n.Normalize("  Hello   World  ")
	if kb.Exact != "Hello   World" {
		t.Errorf("exact: got %q, want %q", kb.Exact, "Hello   World")
	}
	if kb.Norm != "hello world" {
		t.Errorf("norm: got %q, want %q", kb.Norm, "hello world")
	}
	if kb.Alnum != "helloworld" {
		t.Errorf("alnum: got %q, want %q", kb.Alnum, "helloworld")
	}
}

func TestNormalizerNFKC(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	// Full-width Latin → ASCII
	kb := n.Normalize("Ｈｅｌｌｏ")
	if kb.Norm != "hello" {
		t.Errorf("NFKC: got %q, want %q", kb.Norm, "hello")
	}
}

func TestNormalizerDashes(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	kb := n.Normalize("high—speed—camera")
	if kb.Norm != "high-speed-camera" {
		t.Errorf("dash normalization: got %q, want %q", kb.Norm, "high-speed-camera")
	}
}

func TestNormalizerQuotes(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	kb := n.Normalize("display “module”") // smart double quotes
	if kb.Norm != `display "module"` {
		t.Errorf("quote normalization: got %q, want %q", kb.Norm, `display "module"`)
	}
}

func TestNormalizerDottedInitialisms(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	kb := n.Normalize("U.S.A. patent")
	if kb.Norm != "usa patent" {
		t.Errorf("dotted initialism: got %q, want %q", kb.Norm, "usa patent")
	}
}

func TestNormalizerPossessive(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	kb := n.Normalize("patient's monitor")
	if kb.Norm != "patient monitor" {
		t.Errorf("possessive: got %q, want %q", kb.Norm, "patient monitor")
	}
}

func TestNormalizerLeadingArticle(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	tests := []struct{ in, want string }{
		{"the ventilator", "ventilator"},
		{"an alarm", "alarm"},
		{"a display", "display"},
	}
	for _, tt := range tests {
		kb := n.Normalize(tt.in)
		if kb.Norm != tt.want {
			t.Errorf("article %q: got %q, want %q", tt.in, kb.Norm, tt.want)
		}
	}
}

func TestNormalizerSixKeyKinds(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	kb := n.Normalize("Hello World Display Module")

	if kb.Exact != "Hello World Display Module" {
		t.Errorf("exact: got %q", kb.Exact)
	}
	if kb.Norm != "hello world display module" {
		t.Errorf("norm: got %q", kb.Norm)
	}
	if kb.Alnum == "" {
		t.Error("alnum is empty")
	}
	if kb.Sorted == "" {
		t.Error("sorted is empty")
	}
	if kb.Phonetic == "" {
		t.Error("phonetic is empty")
	}
	if kb.Initials == "" {
		t.Error("initials is empty")
	}
	t.Logf("exact=%q norm=%q alnum=%q sorted=%q phonetic=%q initials=%q",
		kb.Exact, kb.Norm, kb.Alnum, kb.Sorted, kb.Phonetic, kb.Initials)
}

func TestNormalizerDeterminism(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	first := n.Normalize("The Patient's Ventilator Display Module v2.0")
	second := n.Normalize("The Patient's Ventilator Display Module v2.0")
	if first != second {
		t.Error("normalizer is not deterministic")
	}
}

func TestNormalizerInitialsKey(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	kb := n.Normalize("ventilator display module")
	if kb.Initials != "VDM" {
		t.Errorf("initials: got %q, want %q", kb.Initials, "VDM")
	}
}

func TestNormalizerPhoneticStable(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	first := n.Normalize("ventilator")
	second := n.Normalize("ventilator")
	if first.Phonetic != second.Phonetic {
		t.Error("phonetic key is not stable")
	}
	if first.Phonetic == "" {
		t.Error("phonetic key is empty")
	}
	t.Logf("phonetic(ventilator) = %q", first.Phonetic)
}

func TestNormalizerChinesePassthrough(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	// Chinese text should pass through NFKC but not be case-folded or de-pluralized.
	kb := n.Normalize("呼吸机 显示模块")
	// CJK characters should be preserved in norm and alnum.
	if kb.Norm != "呼吸机 显示模块" {
		t.Errorf("Chinese norm: got %q", kb.Norm)
	}
	if kb.Alnum != "呼吸机显示模块" {
		t.Errorf("Chinese alnum: got %q", kb.Alnum)
	}
	if kb.Initials != "呼显" {
		t.Errorf("Chinese initials: got %q", kb.Initials)
	}
}

func TestNormalizerToSemidKeyBundle(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	kb := n.Normalize("hello world")
	canonical, alternates := kb.ToSemidKeyBundle()
	if canonical != kb.Norm {
		t.Errorf("canonical: got %q, want %q", canonical, kb.Norm)
	}
	if len(alternates) < 3 {
		t.Errorf("expected at least 3 alternate keys, got %d: %v", len(alternates), alternates)
	}
}

func TestSingularization(t *testing.T) {
	n := KeywordNormalizer{Version: 1}
	tests := []struct{ in, want string }{
		{"indices", "index"},
		{"criteria", "criterion"},
		{"analyses", "analysis"},
		{"children", "child"},
		{"categories", "category"},
	}
	for _, tt := range tests {
		kb := n.Normalize(tt.in)
		if kb.Norm != tt.want {
			t.Errorf("%q: got %q, want %q", tt.in, kb.Norm, tt.want)
		}
	}
}
