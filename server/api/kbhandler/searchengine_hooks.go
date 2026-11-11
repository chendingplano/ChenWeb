package kbhandler

import (
	"context"

	"github.com/chendingplano/deepdoc/server/api/searchengine"
)

// The search engine (server/api/searchengine) cannot import this package, so the pieces that live
// here are registered with it once, when the package loads: the query embedder (configured embedding
// model) and the query-keyword extractor (prompt loading, model, cache, timeout).
func init() {
	searchengine.SetEmbedder(computeQueryEmbedding)
	searchengine.SetKeywordExtractor(func(ctx context.Context, query string, force bool) []string {
		if force {
			on := true
			call := searchCallFrom(ctx)
			call.keywords = &on
			ctx = withSearchCall(ctx, call)
		}
		return searchQueryKeywords(ctx, query)
	})
}
