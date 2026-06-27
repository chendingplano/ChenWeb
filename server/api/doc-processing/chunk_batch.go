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
