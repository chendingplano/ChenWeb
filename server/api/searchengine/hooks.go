package searchengine

import (
	"context"
	"sync"
)

// Embedder embeds one text with the configured model, or reports ok=false (no model, API error,
// wrong dimension). The engine owns the choice of model; callers cannot supply vectors.
type Embedder func(ctx context.Context, text string) (vec []float64, ok bool)

// KeywordExtractor turns a text query into keywords (the search-query-keywords LLM step), or returns
// nil when extraction is off or fails. force is true when the request asked for extraction.
type KeywordExtractor func(ctx context.Context, query string, force bool) []string

var hooks struct {
	mu        sync.RWMutex
	embedder  Embedder
	extractor KeywordExtractor
}

// SetEmbedder registers the query embedder once at startup. Without one, text queries and
// references without a stored embedding run on the keyword list alone.
func SetEmbedder(fn Embedder) {
	hooks.mu.Lock()
	hooks.embedder = fn
	hooks.mu.Unlock()
}

// SetKeywordExtractor registers the query-keyword extractor once at startup. Without one,
// extraction is unavailable and the fallback rules apply. Tests set and reset it.
func SetKeywordExtractor(fn KeywordExtractor) {
	hooks.mu.Lock()
	hooks.extractor = fn
	hooks.mu.Unlock()
}

func currentEmbedder() Embedder {
	hooks.mu.RLock()
	defer hooks.mu.RUnlock()
	return hooks.embedder
}

func currentExtractor() KeywordExtractor {
	hooks.mu.RLock()
	defer hooks.mu.RUnlock()
	return hooks.extractor
}
