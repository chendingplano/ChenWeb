// Package semid implements the canonicalization kernel (ADR 2026072901 DR15):
// one shared mechanism for normalize -> candidates -> score -> adjudicate ->
// link -> merge/split -> audit, instantiated per identity family. Only what
// differs is declared by a FamilyAdapter (DR15: "Family instantiations declare
// only what differs").
package semid

import (
	"context"
	"sort"
)

// NodeCandidate is a canonical node the family offers as a potential match.
type NodeCandidate struct {
	NodeID    string
	KeyBundle KeyBundle
	// Payload carries family-specific detail (e.g. the term row) for the
	// change-set a governed family produces.
	Payload any
}

// FamilyAdapter declares what differs between identity families. The kernel
// owns everything else. A family must never edit the mechanism (DR11 seam 9).
type FamilyAdapter interface {
	// FamilyName is the family key recorded in the decision log
	// (e.g. "ontology_term", "object", "keyword", "category").
	FamilyName() string
	// Normalizer returns the family's normalizer profile (versioned).
	Normalizer() Normalizer
	// AutoAcceptPolicy returns the family's automatic-acceptance gate.
	AutoAcceptPolicy() AutoAcceptPolicy
	// Scope returns the identity scope for a surface (module, knowledge
	// store, plant, org).
	Scope(surface string) string
	// CandidateNodes performs the family's candidate generation for a
	// surface within a scope: the nodes that could match. Blocking/search
	// strategies (exact key, trigram, vector) live here.
	CandidateNodes(ctx context.Context, surface, scope string) ([]NodeCandidate, error)
}

// Resolution is the kernel's deterministic output for one surface.
type Resolution struct {
	Family   string        `json:"family"`
	Scope    string        `json:"scope"`
	Surface  string        `json:"surface"`
	Key      string        `json:"key"`
	Matches  []ScoredMatch `json:"matches"`
	Verdict  Verdict       `json:"verdict"`
	ResolvedNodeID string  `json:"resolved_node_id,omitempty"`
}

// Kernel is the shared canonicalization mechanism.
type Kernel struct {
	Family FamilyAdapter
}

// Resolve runs the kernel over one surface: normalize, generate candidates,
// score deterministically, adjudicate. Linking and change-set application are
// family acts on top of the Resolution.
func (k Kernel) Resolve(ctx context.Context, surface string) (Resolution, error) {
	if k.Family == nil {
		return Resolution{}, errNoFamily
	}
	bundle := k.Family.Normalizer().Normalize(surface)
	scope := k.Family.Scope(surface)
	candidates, err := k.Family.CandidateNodes(ctx, surface, scope)
	if err != nil {
		return Resolution{}, err
	}

	matches := make([]ScoredMatch, 0, len(candidates))
	for _, c := range candidates {
		s := Score(bundle, c.KeyBundle)
		if s > 0 {
			matches = append(matches, ScoredMatch{NodeID: c.NodeID, Score: s, Reason: scoreReason(s)})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].Score > matches[j].Score })

	res := Resolution{
		Family:  k.Family.FamilyName(),
		Scope:   scope,
		Surface: surface,
		Key:     bundle.CanonicalKey,
		Matches: matches,
		Verdict: Adjudicate(matches, k.Family.AutoAcceptPolicy()),
	}
	if res.Verdict == VerdictAutoAccept && len(matches) > 0 {
		res.ResolvedNodeID = matches[0].NodeID
	}
	return res, nil
}

func scoreReason(s float64) string {
	switch s {
	case 1.0:
		return "exact key match"
	case 0.8:
		return "alternate key match"
	case 0.5:
		return "prefix match"
	default:
		return "lexical similarity"
	}
}

var errNoFamily = errorString("semid kernel has no family adapter")

type errorString string

func (e errorString) Error() string { return string(e) }
