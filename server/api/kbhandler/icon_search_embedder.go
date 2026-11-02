package kbhandler

import (
	"context"
	"errors"

	"github.com/chendingplano/shared/go/api/iconlib"
)

// RegisterIconSearchEmbedder gives the shared icon library the same query
// embedder that powers KB search, so icon search can match descriptions
// semantically. Without a configured embedding model, icon search stays BM25-only.
func RegisterIconSearchEmbedder() {
	iconlib.SetQueryEmbedder(func(ctx context.Context, text string) ([]float32, error) {
		vec, ok := computeQueryEmbedding(ctx, text)
		if !ok {
			return nil, errors.New("query embedding unavailable") // iconlib drops the vector leg
		}
		out := make([]float32, len(vec))
		for i, x := range vec {
			out[i] = float32(x)
		}
		return out, nil
	})
}
