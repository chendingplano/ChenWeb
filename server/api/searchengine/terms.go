package searchengine

import (
	"sort"
	"strings"
	"unicode"
)

const (
	maxKeywordStrings = 12
	maxKeywordRunes   = 64
)

// cleanKeywords trims, de-duplicates and bounds keyword strings, as the extraction step does:
// at most 12 strings, none longer than 64 characters, none without a letter, digit or CJK character.
func cleanKeywords(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] || len([]rune(s)) > maxKeywordRunes || !strings.ContainsFunc(s, isTokenRune) {
			continue
		}
		seen[s] = true
		out = append(out, s)
		if len(out) == maxKeywordStrings {
			break
		}
	}
	return out
}

func isTokenRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// frequentTerms picks the most frequent words of a text (Postgres backend, reference queries with no
// stored keywords). Words are runs of letters and digits; CJK runs are kept whole, which the
// full-text dictionary treats as one word, so this is a coarse fallback.
func frequentTerms(text string, n int) []string {
	counts := map[string]int{}
	first := map[string]int{}
	for i, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !isTokenRune(r) }) {
		if len([]rune(w)) < 2 {
			continue
		}
		if _, ok := first[w]; !ok {
			first[w] = i
		}
		counts[w]++
	}
	words := make([]string, 0, len(counts))
	for w := range counts {
		words = append(words, w)
	}
	sort.Slice(words, func(i, j int) bool {
		if counts[words[i]] != counts[words[j]] {
			return counts[words[i]] > counts[words[j]]
		}
		return first[words[i]] < first[words[j]]
	})
	if len(words) > n {
		words = words[:n]
	}
	return words
}
