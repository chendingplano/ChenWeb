package candidates

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

// IdentityKey returns a deterministic, wording-invariant identity signal for
// a term candidate: proposed_module_id + the payload's term_kind + the
// normalized set {label} ∪ aliases (case-folded, trimmed, order-independent)
// -- unlike Fingerprint, it is unaffected by which of two synonymous names
// ends up as label vs. an alias, or by definition/description wording drift
// between extraction passes (design.md Decision 1 of the
// ontology-candidate-dedup change).
//
// It returns "" for any candidateKind other than "term", or when the
// payload has no usable label -- callers store "" as a NULL identity_key,
// meaning "no dedup signal for this row."
func IdentityKey(candidateKind, moduleID string, payload []byte) string {
	if candidateKind != "term" {
		return ""
	}
	var p struct {
		TermKind string   `json:"term_kind"`
		Label    string   `json:"label"`
		Aliases  []string `json:"aliases"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}

	names := map[string]bool{}
	addName := func(s string) {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			names[s] = true
		}
	}
	addName(p.Label)
	for _, alias := range p.Aliases {
		addName(alias)
	}
	if len(names) == 0 {
		return ""
	}

	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)

	digest := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(moduleID),
		strings.TrimSpace(p.TermKind),
		strings.Join(sorted, "\x1e"),
	}, "\x00")))
	return hex.EncodeToString(digest[:])
}
