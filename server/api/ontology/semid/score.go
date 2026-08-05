package semid

import "strings"

// ScoredMatch is a candidate node with a deterministic score, reason, and the
// tier method that produced the candidate.
type ScoredMatch struct {
	NodeID string
	Score  float64
	Reason string
	Method string
}

// Score computes a deterministic similarity between the query's key bundle
// and a candidate's key bundle. Matching is symmetric over the two bundles:
//
//	exact canonical match                      -> 1.0
//	any key of one bundle == any key of the
//	other bundle (canonical or alternate)      -> 0.8
//	mutual canonical prefix, both >= 3 chars   -> 0.5
//	else                                       -> 0
//
// The symmetric alternate branches are what let a query's derived key
// (initials, singular, sorted) match a node's stored alternate key at tiers
// 2 and 4 while candidates keep their honest stored bundles. Lexical
// similarity is evidence only — it never activates identity, class
// membership, or mappings by itself (DR13).
func Score(surface, candidate KeyBundle) float64 {
	s, c := surface.CanonicalKey, candidate.CanonicalKey
	if s == "" && c == "" {
		return 0
	}
	if s != "" && s == c {
		return 1.0
	}
	if s != "" && containsKey(candidate.AlternateKeys, s) {
		return 0.8
	}
	if c != "" && containsKey(surface.AlternateKeys, c) {
		return 0.8
	}
	for _, alt := range surface.AlternateKeys {
		if alt != "" && containsKey(candidate.AlternateKeys, alt) {
			return 0.8
		}
	}
	if s != "" && c != "" && len(s) >= 3 && len(c) >= 3 &&
		(strings.HasPrefix(c, s) || strings.HasPrefix(s, c)) {
		return 0.5
	}
	return 0
}

func containsKey(keys []string, key string) bool {
	for _, k := range keys {
		if k == key {
			return true
		}
	}
	return false
}
