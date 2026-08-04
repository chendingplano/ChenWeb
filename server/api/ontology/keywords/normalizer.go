package keywords

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// KeywordKeyBundle holds the 6 key kinds produced by the keyword normalizer.
// Exact is the verbatim surface; Norm is the fully-normalized form.
type KeywordKeyBundle struct {
	Exact    string
	Norm     string
	Alnum    string
	Sorted   string
	Phonetic string
	Initials string
}

// KeywordNormalizer is a versioned, deterministic surface normalizer for the
// keyword identity family. The full pipeline: NFKC → strip zero-width chars →
// normalize dashes/quotes → collapse whitespace → case-fold → collapse dotted
// initialisms → drop possessive 's → strip leading articles →
// exception-list-aware singularization.
type KeywordNormalizer struct {
	Version int
}

// Normalize applies the full keyword normalization pipeline and returns the
// 6-key bundle.
func (n KeywordNormalizer) Normalize(surface string) KeywordKeyBundle {
	s := norm.NFKC.String(surface)
	s = stripZeroWidth(s)
	s = normalizeDashes(s)
	s = normalizeQuotes(s)
	s = collapseWhitespace(s)
	s = collapseDottedInitialisms(s) // before case-fold: isDottedInitialism checks uppercase
	s = caseFold(s)
	s = dropPossessiveS(s)
	s = stripLeadingArticles(s)
	s = singularize(s)

	normKey := s
	tokens := strings.Fields(normKey) // tokens from norm (has spaces) for sorted/initials
	alnum := alnumKey(normKey)

	return KeywordKeyBundle{
		Exact:    strings.TrimSpace(surface),
		Norm:     normKey,
		Alnum:    alnum,
		Sorted:   sortedKey(tokens),
		Phonetic: phoneticKey(normKey),
		Initials: initialsKey(tokens),
	}
}

// ToSemidKeyBundle maps the keyword key bundle onto the semid kernel's
// KeyBundle: CanonicalKey = Norm, AlternateKeys = [Alnum, Sorted, Phonetic, Initials].
// This mapping ensures the kernel's Score() function naturally produces:
//
//	Tier 1: surface.CanonicalKey == candidate.CanonicalKey (both norm) → 1.0
//	Tier 2: surface.AlternateKeys contains candidate.CanonicalKey (alnum/sorted) → 0.8
//	Tier 4: surface.AlternateKeys contains candidate.CanonicalKey (initials) → 0.8
func (kb KeywordKeyBundle) ToSemidKeyBundle() (canonical string, alternates []string) {
	alternates = make([]string, 0, 4)
	if kb.Alnum != "" {
		alternates = append(alternates, kb.Alnum)
	}
	if kb.Sorted != "" && kb.Sorted != kb.Alnum {
		alternates = append(alternates, kb.Sorted)
	}
	if kb.Phonetic != "" {
		alternates = append(alternates, kb.Phonetic)
	}
	if kb.Initials != "" {
		alternates = append(alternates, kb.Initials)
	}
	return kb.Norm, alternates
}

// -- pipeline steps -----------------------------------------------------------

func stripZeroWidth(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case 0x200B, // zero-width space
			0x200C, // zero-width non-joiner
			0x200D, // zero-width joiner
			0xFEFF, // BOM / zero-width no-break space
			0x200E, // left-to-right mark
			0x200F: // right-to-left mark
			return -1
		}
		return r
	}, s)
}

func normalizeDashes(s string) string {
	// Normalize em-dash, en-dash, figure dash, horizontal bar to ASCII hyphen.
	replacer := strings.NewReplacer(
		"—", "-", // em-dash
		"–", "-", // en-dash
		"‒", "-", // figure dash
		"―", "-", // horizontal bar
	)
	return replacer.Replace(s)
}

func normalizeQuotes(s string) string {
	replacer := strings.NewReplacer(
		"‘", "'", // left single
		"’", "'", // right single
		"“", `"`, // left double
		"”", `"`, // right double
	)
	return replacer.Replace(s)
}

func collapseWhitespace(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			space = true
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func caseFold(s string) string {
	// Use unicode.ToLower for full Unicode case folding. CJK characters are
	// unaffected (they have no case).
	var b strings.Builder
	for _, r := range s {
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func collapseDottedInitialisms(s string) string {
	// "U.S.A." → "usa", "N.A.T.O." → "nato"
	// Only collapse when dots separate single uppercase letters.
	parts := strings.Fields(s)
	for i, p := range parts {
		if isDottedInitialism(p) {
			parts[i] = strings.ToLower(strings.ReplaceAll(p, ".", ""))
		}
	}
	return strings.Join(parts, " ")
}

func isDottedInitialism(s string) bool {
	// Strip trailing dot: "U.S.A." → "U.S.A"
	if strings.HasSuffix(s, ".") {
		s = s[:len(s)-1]
	}
	if len(s) < 3 || !strings.Contains(s, ".") {
		return false
	}
	// Pattern: letter-dot-letter(-dot-letter...)
	for i := 0; i < len(s); i++ {
		if i%2 == 0 {
			if s[i] < 'A' || s[i] > 'Z' {
				return false
			}
		} else {
			if s[i] != '.' {
				return false
			}
		}
	}
	return len(s)%2 == 1 // must end with a letter
}

func dropPossessiveS(s string) string {
	// Drop trailing "'s" from words.
	return strings.ReplaceAll(s, "'s ", " ")
}

func stripLeadingArticles(s string) string {
	// English only: strip "a ", "an ", "the " from the beginning.
	for _, art := range []string{"the ", "an ", "a "} {
		if strings.HasPrefix(s, art) {
			return s[len(art):]
		}
	}
	return s
}

// simpleSingularExceptionList maps common irregular plurals to their singular.
var simpleSingularExceptionList = map[string]string{
	"indices":  "index",
	"matrices": "matrix",
	"vertices": "vertex",
	"axes":     "axis",
	"criteria": "criterion",
	"phenomena": "phenomenon",
	"analyses": "analysis",
	"bases":    "basis",
	"theses":   "thesis",
	"children": "child",
	"people":   "person",
	"men":      "man",
	"women":    "woman",
	"feet":     "foot",
	"teeth":    "tooth",
	"mice":     "mouse",
	"geese":    "goose",
	"lives":    "life",
	"knives":   "knife",
	"wolves":   "wolf",
	"shelves":  "shelf",
}

func singularize(s string) string {
	// Exception-list-aware singularization. Only apply to English words.
	words := strings.Fields(s)
	for i, w := range words {
		if u, ok := simpleSingularExceptionList[w]; ok {
			words[i] = u
		} else if strings.HasSuffix(w, "ies") && len(w) > 4 {
			words[i] = w[:len(w)-3] + "y"
		} else if strings.HasSuffix(w, "ves") && len(w) > 4 {
			words[i] = w[:len(w)-3] + "f"
		} else if strings.HasSuffix(w, "es") && (strings.HasSuffix(w, "ses") || strings.HasSuffix(w, "zes") ||
			strings.HasSuffix(w, "ches") || strings.HasSuffix(w, "shes") || strings.HasSuffix(w, "xes")) {
			words[i] = w[:len(w)-2]
		} else if strings.HasSuffix(w, "s") && !strings.HasSuffix(w, "ss") && len(w) > 3 {
			words[i] = w[:len(w)-1]
		}
	}
	return strings.Join(words, " ")
}

// -- key derivation -----------------------------------------------------------

func alnumKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r >= 0x2E80 { // CJK and beyond: keep
			b.WriteRune(r)
		}
	}
	return b.String()
}

func sortedKey(tokens []string) string {
	if len(tokens) <= 1 {
		return strings.Join(tokens, " ")
	}
	sorted := make([]string, len(tokens))
	copy(sorted, tokens)
	sort.Strings(sorted)
	return strings.Join(sorted, " ")
}

func phoneticKey(s string) string {
	// Stub: first character + first 4 consonants. Full Double Metaphone is
	// deferred (P3 follow-up). This stub satisfies the 6-key-kind contract and
	// produces a deterministic, stable key for tier 4 matching.
	vowels := map[rune]bool{'a': true, 'e': true, 'i': true, 'o': true, 'u': true}
	var first rune
	consonants := make([]rune, 0, 4)
	for _, r := range s {
		if r < 'a' || r > 'z' {
			continue
		}
		if first == 0 {
			first = r
		}
		if !vowels[r] && len(consonants) < 4 {
			consonants = append(consonants, r)
		}
	}
	if first == 0 {
		return ""
	}
	return string(first) + string(consonants)
}

func initialsKey(tokens []string) string {
	var b strings.Builder
	for _, t := range tokens {
		if len(t) > 0 {
			// Use []rune to correctly handle multi-byte CJK characters.
			b.WriteRune(unicode.ToUpper([]rune(t)[0]))
		}
	}
	return b.String()
}
