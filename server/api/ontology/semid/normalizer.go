package semid

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// CurrentNormalizerVersion is the version stamped on every derived key.
// Version 2 reworks version 1's unsafe singularization (spec 2026080403 N1):
// singularization moved out of the canonical key into an alternate key, and
// every language-specific step is guarded by script detection. A version
// change is a re-index, never data loss — keys are derived from
// surface + version (D6).
const CurrentNormalizerVersion = 2

// KeyBundle is the kernel's matching currency: the canonical lookup key plus
// alternate keys. Keys are what Score matches on.
type KeyBundle struct {
	CanonicalKey  string
	AlternateKeys []string
}

// KeySet is the full set of derived keys one surface produces. Norm is the
// canonical key (the tier-1 index); the rest are alternates stored in
// kb.keyword_surface_keys and matched at tiers 2/4. Singular is deliberately
// an alternate, never part of Norm: an over-aggressive singularization rule
// must degrade recall, not destroy identity (spec §6.4 item d).
type KeySet struct {
	Exact    string
	Norm     string
	Alnum    string
	Sorted   string
	Phonetic string
	Initials string
	Singular string
}

// Bundle maps the key set onto the kernel's KeyBundle. Alternates equal to
// the canonical key are omitted — they carry no matching signal.
func (ks KeySet) Bundle() KeyBundle {
	seen := map[string]bool{ks.Norm: true}
	alts := make([]string, 0, 5)
	for _, k := range []string{ks.Alnum, ks.Sorted, ks.Phonetic, ks.Initials, ks.Singular} {
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		alts = append(alts, k)
	}
	return KeyBundle{CanonicalKey: ks.Norm, AlternateKeys: alts}
}

// Normalizer is the one shared surface normalizer (D3). Lexical
// normalization depends on the language of the string, never on which family
// is asking — both the keyword and the term family run this same pipeline.
// Version is metadata stamped on derived keys; the pipeline itself is the
// current single implementation (a version change rebuilds every key).
type Normalizer struct {
	Version int
}

// caseFolder performs full Unicode case folding (§6.3 item 1) — not
// ToLower, which misses multi-character folds. Stateless and shared.
var caseFolder = cases.Fold()

// Normalize applies the normalization pipeline and returns the derived keys:
//
//	NFKC → strip zero-width/soft-hyphen → dashes → quotes → whitespace
//	→ [latin guard: dotted initialisms] → full case fold
//	→ [latin guard: possessives] → [latin guard: leading articles] → Norm
//
// Singular is derived separately from the case-folded tokens and is never
// part of Norm.
func (n Normalizer) Normalize(surface string) KeySet {
	s := norm.NFKC.String(surface)
	s = stripZeroWidth(s)
	s = normalizeDashes(s)
	s = normalizeQuotes(s)
	s = collapseWhitespace(s)

	latin := latinProfile(s)
	if latin {
		// Before case folding: isDottedInitialism checks uppercase.
		s = collapseDottedInitialisms(s)
	}

	// Record the casing signal before folding: singularization must skip any
	// token whose original form carried uppercase (acronyms, proper nouns)
	// — spec §6.4 item (b). Keys are folded token keys.
	caseSignal := map[string]bool{} // folded token -> contained uppercase
	for _, tok := range strings.Fields(s) {
		stripped := stripTokenPossessive(tok)
		key := caseFolder.String(stripped)
		caseSignal[key] = caseSignal[key] || containsUpper(stripped)
	}

	s = caseFolder.String(s)
	if latin {
		s = dropPossessives(s)
		s = stripLeadingArticles(s)
	}

	tokens := strings.Fields(s)
	// F7: singularization is an English rule (irregular plurals, the
	// ss/us/is/ics guards, the suffix rules) — it must stay behind the same
	// latin gate as possessives/articles above. Without it, a mixed
	// CJK/Latin string like "显示屏幕亮度 nits" ran the English suffix rule on
	// "nits" despite latinProfile reporting false for the string as a
	// whole (§6.3 item 2, §6.4 fix item (a) recurring one gate downstream).
	singular := ""
	if latin {
		singular = singularKey(tokens, caseSignal)
	}
	ks := KeySet{
		Exact:    strings.TrimSpace(surface),
		Norm:     s,
		Alnum:    alnumKey(s),
		Sorted:   sortedKey(tokens),
		Phonetic: phoneticKey(s),
		Initials: initialsKey(tokens),
		Singular: singular,
	}
	return ks
}

// latinProfile reports whether the English language profile applies: Latin
// letters are present and at least as numerous as Han characters. Language
// identification is out of scope — script detection is the guard hint
// (spec §7 item 2), and it keeps English rules away from CJK strings
// (spec §6.3 item 2).
func latinProfile(s string) bool {
	latin, han := 0, 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Latin, r):
			latin++
		case unicode.Is(unicode.Han, r):
			han++
		}
	}
	return latin > 0 && latin >= han
}

func containsUpper(s string) bool {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
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
			0x200F, // right-to-left mark
			0x00AD: // soft hyphen
			return -1
		}
		return r
	}, s)
}

func normalizeDashes(s string) string {
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

func collapseDottedInitialisms(s string) string {
	// "U.S.A." → "usa": dots between single uppercase letters.
	parts := strings.Fields(s)
	for i, p := range parts {
		if isDottedInitialism(p) {
			parts[i] = strings.ToLower(strings.ReplaceAll(p, ".", ""))
		}
	}
	return strings.Join(parts, " ")
}

func isDottedInitialism(s string) bool {
	if strings.HasSuffix(s, ".") {
		s = s[:len(s)-1]
	}
	if len(s) < 3 || !strings.Contains(s, ".") {
		return false
	}
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
	return len(s)%2 == 1
}

// stripTokenPossessive drops a trailing "'s" from one token — including the
// word-final position the old "'s " replacement missed (N2: AWS's → aws').
func stripTokenPossessive(tok string) string {
	if strings.HasSuffix(tok, "'s") {
		return tok[:len(tok)-2]
	}
	return tok
}

func dropPossessives(s string) string {
	parts := strings.Fields(s)
	for i, p := range parts {
		parts[i] = stripTokenPossessive(p)
	}
	return strings.Join(parts, " ")
}

func stripLeadingArticles(s string) string {
	for _, art := range []string{"the ", "an ", "a "} {
		if strings.HasPrefix(s, art) {
			return s[len(art):]
		}
	}
	return s
}

// -- singularization (alternate key only) --------------------------------------

// irregularPlurals maps common irregular plurals to their singular.
var irregularPlurals = map[string]string{
	"indices":   "index",
	"matrices":  "matrix",
	"vertices":  "vertex",
	"axes":      "axis",
	"criteria":  "criterion",
	"phenomena": "phenomenon",
	"analyses":  "analysis",
	"bases":     "basis",
	"theses":    "thesis",
	"children":  "child",
	"people":    "person",
	"men":       "man",
	"women":     "woman",
	"feet":      "foot",
	"teeth":     "tooth",
	"mice":      "mouse",
	"geese":     "goose",
	"lives":     "life",
	"knives":    "knife",
	"wolves":    "wolf",
	"shelves":   "shelf",
	"leaves":    "leaf",
	"halves":    "half",
	"calves":    "calf",
	"selves":    "self",
	"elves":     "elf",
	"loaves":    "loaf",
	"thieves":   "thief",
	"wives":     "wife",
	"potatoes":  "potato",
	"heroes":    "hero",
	"echoes":    "echo",
	"tomatoes":  "tomato",
	"zeroes":    "zero",
}

// singularStopList is the hard stop-list of known singulars and invariant
// forms that suffix rules must never touch (spec §6.4 item c). The us/is/ss
// suffix guards cover the largest classes (status, virus, campus, analysis,
// basis, ...); this list catches the rest — including singulars ending in a
// plain "s" the guards do not see (bias, alias, atlas, ...).
var singularStopList = map[string]bool{
	"alias": true, "atlas": true, "bias": true, "asbestos": true,
	"chaos": true, "cosmos": true, "lens": true, "news": true,
	"kudos": true, "pathos": true, "ethos": true, "logos": true,
	"mythos": true, "bathos": true, "movies": true, "zombies": true,
	"cookies": true, "calories": true, "series": true, "species": true,
	"diabetes": true, "mumps": true, "measles": true,
}

// singularKey derives the singularized alternate key. It runs only for
// tokens the English profile applies to and only for tokens whose original
// (pre-fold) form was all-lowercase — acronyms and proper nouns carry no
// plural signal and are never singularized (N1).
func singularKey(tokens []string, caseSignal map[string]bool) string {
	out := make([]string, len(tokens))
	for i, tok := range tokens {
		if caseSignal[tok] {
			out[i] = tok
			continue
		}
		out[i] = singularizeToken(tok)
	}
	return strings.Join(out, " ")
}

func singularizeToken(w string) string {
	if u, ok := irregularPlurals[w]; ok {
		return u
	}
	if singularStopList[w] {
		return w
	}
	// Guards: short tokens, and the large singular classes ending in
	// ss/us/is/ics (status, bias, virus, campus, analysis, basis, physics...).
	if len(w) <= 3 || strings.HasSuffix(w, "ss") || strings.HasSuffix(w, "us") ||
		strings.HasSuffix(w, "is") || strings.HasSuffix(w, "ics") {
		return w
	}
	switch {
	case strings.HasSuffix(w, "zzes"):
		// quizzes/buzzes collide — no suffix rule is safe here.
		return w
	case strings.HasSuffix(w, "sses"):
		return w[:len(w)-2] // classes → class
	case strings.HasSuffix(w, "ches") || strings.HasSuffix(w, "shes") ||
		strings.HasSuffix(w, "xes"):
		return w[:len(w)-2] // watches → watch, boxes → box
	case strings.HasSuffix(w, "ies") && len(w) > 4:
		return w[:len(w)-3] + "y" // categories → category
	case strings.HasSuffix(w, "s"):
		return w[:len(w)-1]
	}
	return w
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
	// deferred (spec §20.1). Deterministic and stable.
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

// initialsKey produces the lower-cased initials of each token (N3: every
// other key is lower-cased; tier 4 looks the query's normalized form up
// against stored initials, never the other way around).
func initialsKey(tokens []string) string {
	var b strings.Builder
	for _, t := range tokens {
		if len(t) > 0 {
			b.WriteRune(unicode.ToLower([]rune(t)[0]))
		}
	}
	return b.String()
}
