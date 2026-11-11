package searchengine

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strconv"
	"strings"
)

const (
	defaultConnectMinCosine = 0.75
	defaultConnectMaxLinks  = 10
)

// ConnectMinCosine is the cosine floor for "similar artifact" lookups (ARTIFACT_CONNECT_MIN_COSINE,
// default 0.75). It was tuned for text-embedding-3-small and needs recalibrating for bge-m3.
func ConnectMinCosine() float64 {
	if f, err := strconv.ParseFloat(strings.TrimSpace(os.Getenv("ARTIFACT_CONNECT_MIN_COSINE")), 64); err == nil && f > 0 {
		return f
	}
	return defaultConnectMinCosine
}

// ConnectMaxLinks caps similar artifacts per lookup (ARTIFACT_CONNECT_MAX_LINKS, default 10).
func ConnectMaxLinks() int {
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("ARTIFACT_CONNECT_MAX_LINKS"))); err == nil && n > 0 {
		return n
	}
	return defaultConnectMaxLinks
}

// SimilarOptions narrow a Similar lookup.
type SimilarOptions struct {
	Filters   Filters
	MinCosine float64 // 0 = ConnectMinCosine()
	Limit     int     // 0 = ConnectMaxLinks()
	// FallbackText is searched when the reference has no kb.search_artifacts row yet (the artifact
	// itself is then excluded by id). Keyword extraction is off for it. Empty = no fallback.
	FallbackText string
}

// Similar finds artifacts similar to an indexed one with the hybrid engine: the reference's own row
// is never returned, Filters apply before ranking, and results below the cosine floor are dropped
// whichever list found them. A reference that is not indexed (and has no FallbackText) yields no
// results, not an error.
func Similar(ctx context.Context, db *sql.DB, ref ArtifactRef, o SimilarOptions) ([]Result, error) {
	if db == nil {
		return nil, errors.New("searchengine: nil db handle")
	}
	if o.MinCosine <= 0 {
		o.MinCosine = ConnectMinCosine()
	}
	if o.Limit <= 0 {
		o.Limit = ConnectMaxLinks()
	}
	req := Request{
		Query:    Query{Ref: &ref},
		Filters:  o.Filters,
		Criteria: Criteria{MinCosine: o.MinCosine},
		Options:  Options{Limit: o.Limit},
	}
	resp, err := Search(ctx, db, LoadConfig(), req)
	if errors.Is(err, ErrReferenceNotIndexed) {
		if strings.TrimSpace(o.FallbackText) == "" {
			return nil, nil
		}
		req.Query = Query{Text: o.FallbackText}
		req.Filters.ExcludeArtifactIDs = append(append([]string(nil), o.Filters.ExcludeArtifactIDs...), ref.ID)
		req.Options.KeywordExtraction = KeywordExtractionOff
		resp, err = Search(ctx, db, LoadConfig(), req)
	}
	if err != nil {
		return nil, err
	}
	return resp.Results, nil
}
