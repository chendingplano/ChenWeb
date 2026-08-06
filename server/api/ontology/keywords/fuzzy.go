package keywords

import (
	"strings"
	"unicode/utf8"
)

// runeLevenshtein computes the Levenshtein edit distance between a and b,
// counting runes (not bytes) so multi-byte CJK characters count as one edit
// unit each — spec §9.2's fuzzy guardrails are stated in character terms.
func runeLevenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// normalizedSimilarity is 1 - (edit distance / longer length), used both as
// tier 5's continuous score (spec §13.1) and as the len>=9 band's own
// similarity>=0.88 guardrail (§9.2). At the length-5, edit-distance-1
// boundary this is exactly 0.8 -- by construction, every candidate that
// survives the §9.2 guardrails already scores at or above KeywordFamily's
// existing 0.8 MinScore, so no separate tier-5 threshold is needed.
func normalizedSimilarity(a, b string) float64 {
	dist := runeLevenshtein(a, b)
	maxLen := utf8.RuneCountInString(a)
	if bl := utf8.RuneCountInString(b); bl > maxLen {
		maxLen = bl
	}
	if maxLen == 0 {
		return 1
	}
	return 1 - float64(dist)/float64(maxLen)
}

// firstRune returns s's first rune, or 0 for an empty string -- used by the
// length-5-to-8 band's "first character must match" rule (§9.2).
func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

// digitsOnly extracts the digit runes from s, in order, discarding everything
// else -- the comparison key for the digit veto.
func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// digitsDiffer implements §9.2's digit veto: "strings differing in any digit
// never match," applied before any threshold. Two strings with no digits at
// all trivially agree (both empty digit sequences) and are not vetoed here.
func digitsDiffer(a, b string) bool {
	return digitsOnly(a) != digitsOnly(b)
}

// negationAffixPrefixes are the negation prefixes named in spec §9.2. "-less"
// is handled as a suffix separately.
var negationAffixPrefixes = []string{"un", "non", "de", "anti"}

// hasNegationAffixMismatch implements §9.2's negation/affix veto: exactly one
// of a/b is the other with a negation prefix or the "-less" suffix attached
// (e.g. "compliant"/"noncompliant", "encrypted"/"unencrypted",
// "harm"/"harmless"). This is deliberately a *pairwise* check -- testing
// "does this affix prefix this one string" in isolation would veto ordinary
// words like "design" or "deploy" that merely start with "de".
func hasNegationAffixMismatch(a, b string) bool {
	for _, p := range negationAffixPrefixes {
		if a == p+b || b == p+a {
			return true
		}
	}
	if a != b {
		if strings.TrimSuffix(a, "less") == b || strings.TrimSuffix(b, "less") == a {
			return true
		}
	}
	return false
}
