package kbhandler

import (
	"context"
	"os"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// embeddingQueryMaxRunes bounds text sent to the embedding API (see the indexer's
// maxEmbeddingRunes); conservative for CJK where tokens ~ runes.
const embeddingQueryMaxRunes = 6000

// newSearchQueryEmbedder builds the embedding client from EMBEDDING_MODEL_NAME +
// .models.toml (same resolution as the graph filter). ok=false when no embedding
// model is configured. Build once and reuse across many Embed calls (e.g. backfill).
func newSearchQueryEmbedder() (client *llmclients.OpenAIJSONClient, modelName string, ok bool) {
	modelRef := strings.TrimSpace(os.Getenv("EMBEDDING_MODEL_NAME"))
	if modelRef == "" {
		return nil, "", false
	}
	cfg, found := resolveGraphFilterEmbeddingConfig(modelRef)
	if !found || strings.TrimSpace(cfg.ModelName) == "" {
		return nil, "", false
	}
	return newKBEmbeddingClient(
		modelRef,
		cfg,
		60,
		kbsearch.EmbeddingDimensionsForModel(cfg.ModelName, cfg.BaseURL),
	), cfg.ModelName, true
}

// computeQueryEmbedding embeds the search query for the semantic half of hybrid
// search. It is best-effort: ok=false (empty model, config miss, API error, or
// dimension mismatch) tells the caller to fall back to lexical-only search.
func computeQueryEmbedding(ctx context.Context, queryText string) ([]float64, bool) {
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return nil, false
	}
	client, modelName, ok := newSearchQueryEmbedder()
	if !ok {
		return nil, false
	}
	call := searchCallFrom(ctx)
	vec, err := client.Embed(ctx, llmclients.EmbedInput{
		UserID:     call.userID,
		RunID:      call.runID,
		ModelName:  modelName,
		InputText:  truncateRunesForEmbedding(queryText, embeddingQueryMaxRunes),
		CallReason: "search_query_embedding",
		CallLoc:    "MID-20261024-02",
	})
	if err != nil {
		return nil, false
	}
	if len(vec) != kbsearch.ConfiguredEmbeddingDim() {
		return nil, false
	}
	return vec, true
}

type precomputedEmbeddingKey struct{}

type precomputedEmbedding struct {
	text string
	vec  []float64
}

// withPrecomputedEmbedding lets a caller that already embedded a query (the benchmark times the embedding
// call itself) hand the vector to the searches that follow, so the model is not called again. It applies
// to that exact text only.
func withPrecomputedEmbedding(ctx context.Context, text string, vec []float64) context.Context {
	return context.WithValue(ctx, precomputedEmbeddingKey{}, precomputedEmbedding{text: strings.TrimSpace(text), vec: vec})
}

// searchQueryEmbedding is the query embedder the search engine calls: a precomputed vector for the
// text when the context carries one, else the configured model.
func searchQueryEmbedding(ctx context.Context, queryText string) ([]float64, bool) {
	if pre, ok := ctx.Value(precomputedEmbeddingKey{}).(precomputedEmbedding); ok && pre.text == strings.TrimSpace(queryText) {
		return pre.vec, len(pre.vec) > 0
	}
	return computeQueryEmbedding(ctx, queryText)
}

func truncateRunesForEmbedding(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
