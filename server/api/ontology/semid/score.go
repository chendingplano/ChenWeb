package semid

import "strings"

// ScoredMatch is a candidate node with a deterministic score and reason.
type ScoredMatch struct {
	NodeID string
	Score  float64
	Reason string
}

// Score computes a deterministic similarity between a surface key bundle and
// a candidate's key bundle. Lexical similarity is evidence only -- it never
// activates identity, class membership, or mappings by itself (DR13).
func Score(surface, candidate KeyBundle) float64 {
	s, c := surface.CanonicalKey, candidate.CanonicalKey
	if s == "" || c == "" {
		return 0
	}
	if s == c {
		return 1.0
	}
	for _, alt := range surface.AlternateKeys {
		if alt == c {
			return 0.8
		}
	}
	if strings.HasPrefix(c, s) || strings.HasPrefix(s, c) {
		if len(c) >= 3 && len(s) >= 3 {
			return 0.5
		}
	}
	return 0
}
