package keywords

import "testing"

func TestRuneLevenshtein(t *testing.T) {
	cases := []struct{ a, b string; want int }{
		{"kubernets", "kubernetes", 1},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"显示亮度", "显示屏幕亮度", 2},
	}
	for _, c := range cases {
		if got := runeLevenshtein(c.a, c.b); got != c.want {
			t.Errorf("runeLevenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestNormalizedSimilarity(t *testing.T) {
	// worst case at the length-5 boundary with edit distance 1 must be
	// exactly 0.8 -- the floor that keeps every tier5 candidate at or above
	// KeywordFamily's existing 0.8 MinScore without a separate threshold.
	if got := normalizedSimilarity("abcde", "abcdf"); got != 0.8 {
		t.Errorf("normalizedSimilarity boundary case = %v, want 0.8", got)
	}
	if got := normalizedSimilarity("kubernets", "kubernetes"); got < 0.88 {
		t.Errorf("normalizedSimilarity(kubernets,kubernetes) = %v, want >= 0.88", got)
	}
}

func TestDigitsDiffer(t *testing.T) {
	if !digitsDiffer("cd/m2", "cd/m3") {
		t.Error("cd/m2 vs cd/m3 should veto: digits differ")
	}
	if digitsDiffer("v2 engine", "v2engine") {
		t.Error("same digits, different spacing should not veto")
	}
	if digitsDiffer("kubernets", "kubernetes") {
		t.Error("neither string has a digit; must not veto")
	}
}

func TestHasNegationAffixMismatch(t *testing.T) {
	if !hasNegationAffixMismatch("compliant", "noncompliant") {
		t.Error("compliant vs noncompliant must veto")
	}
	if !hasNegationAffixMismatch("encrypted", "unencrypted") {
		t.Error("encrypted vs unencrypted must veto")
	}
	if !hasNegationAffixMismatch("harmful", "harmless") {
		t.Error("harmful vs harmless must veto")
	}
	if hasNegationAffixMismatch("design", "deploy") {
		t.Error("design vs deploy must not veto -- 'de' is not a real negation prefix here")
	}
	if hasNegationAffixMismatch("kubernets", "kubernetes") {
		t.Error("kubernets vs kubernetes must not veto")
	}
}

func TestFirstRune(t *testing.T) {
	if firstRune("kubernetes") != 'k' {
		t.Error("expected 'k'")
	}
	if firstRune("") != 0 {
		t.Error("expected 0 for empty string")
	}
}
