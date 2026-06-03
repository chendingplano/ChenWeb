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
	return &llmclients.OpenAIJSONClient{
		ModelName: cfg.ModelName,
		APIKey:    cfg.APIKey,
		BaseURL:   cfg.BaseURL,
	}, cfg.ModelName, true
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
	vec, err := client.Embed(ctx, llmclients.EmbedInput{
		ModelName: modelName,
		InputText: truncateRunesForEmbedding(queryText, embeddingQueryMaxRunes),
	})
	if err != nil {
		return nil, false
	}
	if len(vec) != kbsearch.EmbeddingDim {
		return nil, false
	}
	return vec, true
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
