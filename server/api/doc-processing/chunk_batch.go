package docprocessing

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"
)

// ChunkBatchProcessor is implemented by doc processors that participate in
// per-chunk batching across all processors (ADR 2026062701, Phase 3 redesign).
// Instead of each processor iterating over all chunks independently (the old
// approach, which scattered identical chunk prefixes in time), the per-chunk
// batching coordinator calls ProcessChunk once per chunk per processor with a
// configurable stagger between calls, so LLM calls across different processors
// for the same chunk arrive at DeepSeek back-to-back and benefit from implicit
// prompt caching.
//
// The execution order for a document is:
//
//	InitChunkBatch(ctx, recordID, chunks, docCtx)  — once per processor
//	Phase 1: for each chunk index 0..N-1:
//	    processor[0].ProcessChunk(ctx, chunkIdx)     — seeds DeepSeek cache
//	Phase 2: wait LLM_CALL_STAGGER                   — time for cache to persist
//	Phase 3: for each chunk, for each processor[1..M]:
//	    go P.ProcessChunk(ctx, chunkIdx)              — concurrent, benefit from cache
//	for each processor P:
//	    P.FinalizeChunkBatch(ctx)                    — save results
type ChunkBatchProcessor interface {
	// Name returns the processor name (e.g. "extract_provisions").
	Name() string

	// InitChunkBatch initialises the processor for a batch run. It receives
	// the record ID, the pre-loaded shared chunks, and the document context
	// string. It must not make LLM calls. Returns an error if setup fails
	// (e.g. missing model config).
	InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error

	// ProcessChunk processes a single chunk index: builds the LLM input,
	// makes the LLM call (which benefits from the cache populated by previous
	// processors in this batch), and accumulates the result internally.
	// Called sequentially by the coordinator, one processor at a time, with
	// LLM_CALL_STAGGER between the end of one call and the start of the next.
	ProcessChunk(ctx context.Context, chunkIdx int) error

	// FinalizeChunkBatch runs after all chunks have been processed. It saves
	// accumulated results to the database, writes artifact files, indexes
	// search registries, etc. Returns an error if any step fails.
	FinalizeChunkBatch(ctx context.Context) error
}

// llmCallStagger returns the duration to wait between sequential per-chunk
// LLM calls across different processors. Default 1 second. Controlled by
// the LLM_CALL_STAGGER env var (value in seconds).
func llmCallStagger() time.Duration {
	v := strings.TrimSpace(os.Getenv("LLM_CALL_STAGGER"))
	if v == "" {
		return time.Second
	}
	sec, err := strconv.Atoi(v)
	if err != nil || sec < 0 {
		return time.Second
	}
	return time.Duration(sec) * time.Second
}

// multiPassProcessors are batch processors that make more than one LLM call
// per chunk. They must not be chosen as the cache seed (Task: seed selection);
// the seed should plant each chunk's prefix with a single clean call.
var multiPassProcessors = map[string]struct{}{
	"extract_metrics":              {},
	"extract_semantic_projections": {},
}

// maxDocProcessorTasks returns the Phase-3 concurrency cap. Controlled by
// MAX_DOC_PROCESSOR_TASKS (int >= 1); falls back to fallback (min 1). Mirrors
// the reviewers' MAX_DOC_REVIEWER_TASKS.
func maxDocProcessorTasks(fallback int) int {
	if fallback < 1 {
		fallback = 1
	}
	v := strings.TrimSpace(os.Getenv("MAX_DOC_PROCESSOR_TASKS"))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

// orderBatchProcessorsSeedFirst returns a copy of bp with a single-pass
// processor first so it can seed the DeepSeek cache with one call per chunk.
// Stable for all other positions. If every processor is multi-pass, the input
// order is returned unchanged.
func orderBatchProcessorsSeedFirst(bp []ChunkBatchProcessor) []ChunkBatchProcessor {
	out := make([]ChunkBatchProcessor, 0, len(bp))
	seedIdx := -1
	for i, p := range bp {
		if _, multi := multiPassProcessors[p.Name()]; !multi {
			seedIdx = i
			break
		}
	}
	if seedIdx <= 0 {
		return append(out, bp...) // already seed-first, or all multi-pass
	}
	out = append(out, bp[seedIdx])
	for i, p := range bp {
		if i == seedIdx {
			continue
		}
		out = append(out, p)
	}
	return out
}
