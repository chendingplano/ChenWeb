package docreviews

import (
	"context"
	"fmt"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

// searchCandidates keeps the best keyword-only and vector-only passages visible even
// when they fall below the fused page. ground calls it separately per type and query.
func (r *complianceReviewer) searchCandidates(ctx context.Context, req searchengine.Request) ([]searchengine.Result, error) {
	req.Options.Trace = true
	resp, err := r.search(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, fmt.Errorf("compliance search returned a nil response")
	}
	// Trace lists precede criteria filtering. Never reintroduce a rejected passage.
	filtered := req.Criteria.MinCosine > 0 || req.Criteria.MinMatched > 0
	for _, kw := range req.Criteria.Keywords {
		filtered = filtered || kw.Required || kw.Excluded
	}
	if resp.Trace == nil || filtered {
		return resp.Results, nil
	}
	limit := req.Options.Limit
	if limit <= 0 {
		limit = searchengine.DefaultLimit
	}
	key := func(typ, id string) string { return typ + "/" + id }
	loaded := map[string]searchengine.Result{}
	lists := make([][]searchengine.ArtifactRef, 3)
	for _, result := range resp.Results {
		k := key(result.ArtifactType, result.ArtifactID)
		loaded[k] = result
		lists[0] = append(lists[0], searchengine.ArtifactRef{Type: result.ArtifactType, ID: result.ArtifactID})
	}
	diags := map[string]searchengine.Diagnostics{}
	for i, hit := range resp.Trace.Lexical {
		k := key(hit.ArtifactType, hit.ArtifactID)
		diag := diags[k]
		rank, score := i+1, hit.Score
		diag.LexicalRank, diag.LexicalScore = &rank, &score
		diags[k] = diag
		lists[1] = append(lists[1], searchengine.ArtifactRef{Type: hit.ArtifactType, ID: hit.ArtifactID})
	}
	for i, hit := range resp.Trace.Vector {
		k := key(hit.ArtifactType, hit.ArtifactID)
		diag := diags[k]
		rank, cos := i+1, hit.Score
		diag.VectorRank, diag.Cosine = &rank, &cos
		diags[k] = diag
		lists[2] = append(lists[2], searchengine.ArtifactRef{Type: hit.ArtifactType, ID: hit.ArtifactID})
	}
	seen := map[string]bool{}
	var order, missing []searchengine.ArtifactRef
	for rank := 0; len(order) < limit; rank++ {
		more := false
		for _, list := range lists {
			if rank >= len(list) {
				continue
			}
			more = true
			ref := list[rank]
			k := key(ref.Type, ref.ID)
			if seen[k] {
				continue
			}
			seen[k] = true
			order = append(order, ref)
			if _, ok := loaded[k]; !ok {
				missing = append(missing, ref)
			}
			if len(order) == limit {
				break
			}
		}
		if !more {
			break
		}
	}
	if len(missing) > 0 {
		if r.loadArtifacts == nil {
			return nil, fmt.Errorf("compliance candidate artifact loader is not configured")
		}
		results, err := r.loadArtifacts(ctx, missing)
		if err != nil {
			return nil, fmt.Errorf("load compliance candidate artifacts: %w", err)
		}
		for _, result := range results {
			loaded[key(result.ArtifactType, result.ArtifactID)] = result
		}
	}
	fused := map[string]searchengine.TraceFused{}
	for _, hit := range resp.Trace.Fused {
		fused[key(hit.ArtifactType, hit.ArtifactID)] = hit
	}
	out := make([]searchengine.Result, 0, len(order))
	for _, ref := range order {
		k := key(ref.Type, ref.ID)
		result, ok := loaded[k]
		if !ok {
			return nil, fmt.Errorf("compliance candidate %s is no longer in the search index", k)
		}
		diag := diags[k]
		if result.Diag.LexicalRank == nil {
			result.Diag.LexicalRank = diag.LexicalRank
		}
		if result.Diag.LexicalScore == nil {
			result.Diag.LexicalScore = diag.LexicalScore
		}
		if result.Diag.VectorRank == nil {
			result.Diag.VectorRank = diag.VectorRank
		}
		if result.Diag.Cosine == nil {
			result.Diag.Cosine = diag.Cosine
		}
		if result.Score == 0 {
			result.Score = fused[k].Score
		}
		out = append(out, result)
	}
	return out, nil
}
