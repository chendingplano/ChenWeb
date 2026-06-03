package docprocessing

import (
	"context"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
)

// maxEmbeddingRunes bounds the text sent to the embedding API. text-embedding-3-small
// accepts up to ~8191 tokens; for CJK text tokens roughly track runes, so a
// conservative rune budget avoids request failures on very large artifacts.
const maxEmbeddingRunes = 6000

// newSearchEmbedder builds an Embedder from EMBEDDING_MODEL_NAME (+ .models.toml),
// mirroring the chunking service's resolution. Returns ok=false when no embedding
// model is configured, in which case the caller proceeds lexical-only.
func newSearchEmbedder() (embedder Embedder, modelName string, ok bool) {
	if strings.TrimSpace(os.Getenv("EMBEDDING_MODEL_NAME")) == "" {
		return nil, "", false
	}
	_, _, cfg, err := loadModelConfigFromEnv("EMBEDDING_MODEL_NAME", "")
	if err != nil || strings.TrimSpace(cfg.ModelName) == "" {
		return nil, "", false
	}
	timeoutSec := cfg.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	client := &llmclients.OpenAIJSONClient{
		HTTPClient: &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
		ModelName:  cfg.ModelName,
		APIKey:     cfg.APIKey,
		BaseURL:    cfg.BaseURL,
	}
	return client, cfg.ModelName, true
}

// embedRegistryRows fills EmbeddingText + Embedding on each row in place, best-effort.
// Embedding is never fatal to reindexing: a missing model, per-row API failures, and
// dimension mismatches are logged and leave that row lexical-only (NULL embedding).
func embedRegistryRows(ctx context.Context, rows []kbsearch.RegistryRow, logger ApiTypes.JimoLogger) {
	if len(rows) == 0 {
		return
	}
	embedder, modelName, ok := newSearchEmbedder()
	if !ok {
		if logger != nil {
			logger.Warn("semantic search enabled but no embedding model configured; indexing lexical-only",
				"env", "EMBEDDING_MODEL_NAME")
		}
		return
	}
	for i := range rows {
		text := strings.TrimSpace(rows[i].EmbeddingText)
		if text == "" {
			text = strings.TrimSpace(rows[i].SearchDocument)
		}
		if text == "" {
			continue
		}
		text = truncateRunes(text, maxEmbeddingRunes)
		vec, err := embedder.Embed(ctx, llmclients.EmbedInput{ModelName: modelName, InputText: text})
		if err != nil {
			if logger != nil {
				logger.Warn("embedding failed; row indexed lexical-only",
					"artifact_type", rows[i].ArtifactType,
					"artifact_id", rows[i].ArtifactID,
					"error", err.Error())
			}
			continue
		}
		if len(vec) != kbsearch.EmbeddingDim {
			if logger != nil {
				logger.Warn("embedding dimension mismatch; row indexed lexical-only",
					"artifact_type", rows[i].ArtifactType,
					"artifact_id", rows[i].ArtifactID,
					"got_dim", len(vec),
					"want_dim", kbsearch.EmbeddingDim)
			}
			continue
		}
		rows[i].EmbeddingText = text
		rows[i].Embedding = vec
	}
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}
