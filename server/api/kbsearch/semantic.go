package kbsearch

import (
	"os"
	"strconv"
	"strings"
)

// EmbeddingDim is the dimensionality of the embedding vectors stored in
// kb.search_artifacts.embedding. It must match the vector(N) type in the
// pgvector migration (20260603000001) and the configured EMBEDDING_MODEL_NAME
// (gpt-embedding-small / text-embedding-3-small = 1536).
const EmbeddingDim = 1536

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
