package semid

import "strings"

// KeyBundle is the deterministic output of a normalizer: the canonical lookup
// key plus alternate keys. Keys are what the kernel matches on. A normalizer
// version change is a re-index, never data loss (ADR kernel test 18).
type KeyBundle struct {
	CanonicalKey  string
	AlternateKeys []string
}

// Normalizer is a versioned, deterministic surface normalizer. Families
// declare a normalizer profile; the kernel runs it over every surface.
type Normalizer struct {
	Name    string
	Version int
}

// Normalize turns a surface into its key bundle. Version 1 is the basic
// profile (lowercase, trim, collapse whitespace). Version 2 additionally
// strips punctuation. A version change re-indexes the keys -- surfaces and
// links are preserved, only the keys change (ADR kernel test 18).
func (n Normalizer) Normalize(surface string) KeyBundle {
	s := strings.ToLower(strings.TrimSpace(surface))
	if n.Version >= 2 {
		s = stripPunct(s)
	}
	return KeyBundle{CanonicalKey: collapseSpace(s)}
}

// stripPunct removes characters that are not letters, digits, or CJK.
func stripPunct(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 0x2E80: // CJK and beyond: keep
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func collapseSpace(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !space && b.Len() > 0 {
				b.WriteRune(' ')
			}
			space = true
			continue
		}
		space = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}
