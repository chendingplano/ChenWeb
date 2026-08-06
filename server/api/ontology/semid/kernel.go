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
// KeyBundle carries the node's honest stored keys: CanonicalKey is the node's
// stored norm key, AlternateKeys its stored alternate keys. Method records
// the tier that produced the candidate ("tier0_exact", "tier1_norm",
// "tier2_altkey", "tier3_rewrite", "tier4_initials").
type NodeCandidate struct {
	NodeID    string
	KeyBundle KeyBundle
	// Payload carries family-specific detail (e.g. the term row) for the
	// change-set a governed family produces.
	Payload any
	Method  string
	// PrecomputedScore carries a continuous similarity score the family
	// computed itself (tier 5 edit distance, tier 6 embedding cosine) for
	// evidence that isn't a shared derived key and so can't be scored by
	// Score(surface, candidate KeyBundle) alone (spec 2026080403 §13.1).
	// nil for tiers 0-4, whose evidence is a shared key and is scored by
	// Score as before.
	PrecomputedScore *float64
}

// FamilyAdapter declares what differs between identity families. The kernel
// owns everything else. A family must never edit the mechanism (DR11 seam 9).
// Per D2/D3 the interface carries CandidateNodes, AutoAcceptPolicy and
// FamilyName — not Normalizer() (normalization does not differ, D3) and not
// Scope() (scope is the caller's input to Resolve, K2).
type FamilyAdapter interface {
	// FamilyName is the family key recorded in the decision log
	// (e.g. "ontology_term", "keyword").
	FamilyName() string
	// AutoAcceptPolicy returns the family's automatic-acceptance gate.
	AutoAcceptPolicy() AutoAcceptPolicy
	// CandidateNodes performs the family's candidate generation for an input
	// within the caller's scope: the nodes that could match. Blocking/search
	// strategies (exact key, alternate keys, trigram, vector) live here.
	CandidateNodes(ctx context.Context, input, scope string) ([]NodeCandidate, error)
}

// Resolution is the kernel's deterministic output for one input.
type Resolution struct {
	Family  string        `json:"family"`
	Scope   string        `json:"scope"`
	Input   string        `json:"input"`
	Key     string        `json:"key"`
	Matches []ScoredMatch `json:"matches"`
	Verdict Verdict       `json:"verdict"`
	// ResolvedNodeID carries the top-1 node whenever matches exist: on
	// auto_accepted, on ambiguous (top-1 plus the tied set, §8.3/D11) and on
	// human_review (flagged for sampling). Empty on deferred.
	ResolvedNodeID string `json:"resolved_node_id,omitempty"`
	// Method is the tier that produced the carried top-1 node (D11: every
	// decision attributable).
	Method string `json:"method,omitempty"`
}

// Kernel is the shared canonicalization mechanism. The Normalizer is the one
// shared pipeline (D3); the family decides only which version its stored keys
// carry.
type Kernel struct {
	Family     FamilyAdapter
	Normalizer Normalizer
}

// Resolve runs the kernel over one input in the caller's scope: normalize,
// generate candidates, score deterministically, adjudicate. Scope is an
// explicit input, never derived from the family or the input (K2). Linking
// and change-set application are family acts on top of the Resolution.
func (k Kernel) Resolve(ctx context.Context, input, scope string) (Resolution, error) {
	if k.Family == nil {
		return Resolution{}, errNoFamily
	}
	res := Resolution{
		Family: k.Family.FamilyName(),
		Scope:  scope,
		Input:  input,
	}

	bundle := k.Normalizer.Normalize(input).Bundle()
	res.Key = bundle.CanonicalKey

	candidates, err := k.Family.CandidateNodes(ctx, input, scope)
	if err != nil {
		return res, err
	}

	// Zero-scoring candidates are dropped before adjudication (§8.2).
	matches := make([]ScoredMatch, 0, len(candidates))
	for _, c := range candidates {
		s := Score(bundle, c.KeyBundle)
		if c.PrecomputedScore != nil {
			s = *c.PrecomputedScore
		}
		if s > 0 {
			matches = append(matches, ScoredMatch{NodeID: c.NodeID, Score: s, Reason: scoreReason(s), Method: c.Method})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].NodeID < matches[j].NodeID
	})

	res.Matches = matches
	res.Verdict = Adjudicate(matches, k.Family.AutoAcceptPolicy())
	if res.Verdict != VerdictDeferred && len(matches) > 0 {
		res.ResolvedNodeID = matches[0].NodeID
		res.Method = matches[0].Method
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
