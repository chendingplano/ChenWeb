package kbsearch

import (
	"os"
	"strconv"
	"strings"
)

// EmbeddingDim is the default dimensionality of the embedding vectors stored in
// kb.search_artifacts.embedding. It should match the vector(N) type in the
// pgvector migration unless EMBEDDING_DIMENSIONS is intentionally overridden to
// a compatible value.
const EmbeddingDim = 1536
const QwenCloudEmbeddingMaxBatchItems = 10
const QwenCloudEmbeddingMaxBatchRunes = 30000

// SemanticSearchEnabled reports whether hybrid (lexical + pgvector semantic)
// search is turned on. It is OFF by default so the feature can be merged and
// deployed before pgvector is installed and the migration applied; flip
// SEARCH_SEMANTIC_ENABLED=true only after both are in place. When false, the
// write and read paths reference no embedding columns and behave exactly as the
// original lexical-only system.
func SemanticSearchEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SEARCH_SEMANTIC_ENABLED"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// ConfiguredEmbeddingDim returns the requested embedding dimensionality from
// EMBEDDING_DIMENSIONS, falling back to the historical default when unset or
// invalid.
func ConfiguredEmbeddingDim() int {
	raw := strings.TrimSpace(os.Getenv("EMBEDDING_DIMENSIONS"))
	if raw == "" {
		return EmbeddingDim
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return EmbeddingDim
	}
	return n
}

// EmbeddingDimensionsForModel returns the provider-specific output dimension to
// request from the embedding API. Zero means "use provider default".
func EmbeddingDimensionsForModel(modelName, baseURL string) int {
	if isQwenCloudEmbeddingModel(modelName, baseURL) {
		return ConfiguredEmbeddingDim()
	}
	return 0
}

// EmbeddingMaxBatchItemsForModel returns the maximum safe number of texts to
// send in one embedding request for the configured provider. Zero means the
// caller can choose its own batching.
func EmbeddingMaxBatchItemsForModel(modelName, baseURL string) int {
	if isQwenCloudEmbeddingModel(modelName, baseURL) {
		return QwenCloudEmbeddingMaxBatchItems
	}
	return 0
}

// EmbeddingMaxBatchRunesForModel returns a conservative rune-budget for a
// single embedding request. We use runes as a token proxy because CJK-heavy
// content tends to track tokens closely enough to stay under provider limits.
func EmbeddingMaxBatchRunesForModel(modelName, baseURL string) int {
	if isQwenCloudEmbeddingModel(modelName, baseURL) {
		return QwenCloudEmbeddingMaxBatchRunes
	}
	return 0
}

func isQwenCloudEmbeddingModel(modelName, baseURL string) bool {
	model := strings.ToLower(strings.TrimSpace(modelName))
	base := strings.ToLower(strings.TrimSpace(baseURL))
	return model == "text-embedding-v4" || strings.Contains(base, "dashscope.aliyuncs.com")
}

// FormatVectorLiteral renders a vector as the pgvector text literal "[v1,v2,...]"
// for binding to a $N::vector placeholder. Returns "" for an empty vector.
func FormatVectorLiteral(vec []float64) string {
	if len(vec) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.Grow(len(vec) * 12)
	sb.WriteByte('[')
	for i, v := range vec {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
	}
	sb.WriteByte(']')
	return sb.String()
}
