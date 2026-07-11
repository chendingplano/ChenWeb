package docprocessing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type phaseBPartition struct {
	batch       []ChunkBatchProcessor
	unsupported []Processor
}

// partitionBatchProcessors splits Phase B processors (Phase A excluded) into
// those implementing ChunkBatchProcessor and those that don't.
func partitionBatchProcessors(processors []Processor) phaseBPartition {
	var part phaseBPartition
	for _, p := range processors {
		if p == nil || isPhaseAProcessor(p.Name()) {
			continue
		}
		if bp, ok := p.(ChunkBatchProcessor); ok {
			part.batch = append(part.batch, bp)
		} else {
			part.unsupported = append(part.unsupported, p)
		}
	}
	return part
}

// runProcessorsChunkBatched runs Phase B processors using the per-chunk
// batching coordinator. It replaces runProcessorsTwoPhase when all Phase B
// processors implement ChunkBatchProcessor.
//
// Execution:
//  1. Load shared chunks and doc context once.
//  2. InitChunkBatch on each processor.
//  3. For each chunk batch, iterate processors sequentially with LLM_CALL_STAGGER.
//  4. FinalizeChunkBatch on each processor.
//
// Only the first processor error is surfaced; subsequent processors still run
// so their results are available for Phase C post-processing.
func (s *ControlService) runProcessorsChunkBatched(
	ctx context.Context,
	payload []byte,
	processors []Processor,
	recordID int64,
	requestFailed, requestStopped *bool,
	firstErr *error,
	_ *[]procResult,
) {
	part := partitionBatchProcessors(processors)
	batchProcessors := part.batch
	if len(batchProcessors) == 0 {
		return
	}

	batchAsProcs := make([]Processor, len(batchProcessors))
	for i, bp := range batchProcessors {
		batchAsProcs[i] = bp.(Processor)
	}
	_, phaseBSpan := startPhaseSpan(ctx, "B", recordID, batchAsProcs)
	defer phaseBSpan.End()

	// --- shared chunk loading ---
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("chunk batch: parse event failed", "record_id", recordID, "error", err)
		}
		*requestFailed = true
		*firstErr = fmt.Errorf("(MID_26062701) chunk batch parse event: %w", err)
		phaseBSpan.SetStatus(codes.Error, (*firstErr).Error())
		recordError(phaseBSpan, err)
		return
	}
	ctx = withDocProcessorFlags(ctx, evt.Force, evt.ForceClear)
	rec, err := s.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("chunk batch: load record failed", "record_id", recordID, "error", err)
		}
		*requestFailed = true
		*firstErr = fmt.Errorf("(MID_26062702) chunk batch load record: %w", err)
		phaseBSpan.SetStatus(codes.Error, (*firstErr).Error())
		recordError(phaseBSpan, err)
		return
	}
	docCtx := buildDocContextLine(rec)

	lineFilePath, lineFileErr := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if lineFileErr != nil {
		if s.Logger != nil {
			s.Logger.Error("chunk batch: resolve input file path failed", "record_id", recordID, "error", lineFileErr)
		}
		*requestFailed = true
		*firstErr = fmt.Errorf("(MID_26062703) chunk batch resolve input: %w", lineFileErr)
		phaseBSpan.SetStatus(codes.Error, (*firstErr).Error())
		recordError(phaseBSpan, lineFileErr)
		return
	}
	body, readErr := os.ReadFile(lineFilePath)
	if readErr != nil {
		if s.Logger != nil {
			s.Logger.Error("chunk batch: read input file failed", "record_id", recordID, "path", lineFilePath, "error", readErr)
		}
		*requestFailed = true
		*firstErr = fmt.Errorf("(MID_26062704) chunk batch read input: %w", readErr)
		phaseBSpan.SetStatus(codes.Error, (*firstErr).Error())
		recordError(phaseBSpan, readErr)
		return
	}
	lines, parseErr := ParseInputLinesIncludingTOC(body)
	if parseErr != nil {
		if s.Logger != nil {
			s.Logger.Error("chunk batch: parse input lines failed", "record_id", recordID, "error", parseErr)
		}
		*requestFailed = true
		*firstErr = fmt.Errorf("(MID_26062705) chunk batch parse lines: %w", parseErr)
		phaseBSpan.SetStatus(codes.Error, (*firstErr).Error())
		recordError(phaseBSpan, parseErr)
		return
	}

	// Try context buffer first; fall back to artifact file.
	chunks := loadChunksFromBufferOrArtifact(ctx, rec, lines, recordID)
	if len(chunks) == 0 {
		if s.Logger != nil {
			s.Logger.Error("chunk batch: no chunks found", "record_id", recordID)
		}
		*requestFailed = true
		*firstErr = fmt.Errorf("(MID_26062706) no chunks for record_id=%d", recordID)
		phaseBSpan.SetStatus(codes.Error, (*firstErr).Error())
		return
	}

	if s.Logger != nil {
		s.Logger.Info("chunk batch: starting per-chunk batching",
			"record_id", recordID,
			"num_processors", len(batchProcessors),
			"num_chunks", len(chunks),
		)
	}

	// --- Init each processor ---
	var batchFirstErr error

	for _, bp := range batchProcessors {
		if isCtxStopped(ctx) {
			*requestStopped = true
			phaseBSpan.SetStatus(codes.Error, "stopped")
			return
		}
		recCtx := withLLMRecordID(ctx, recordID)
		if err := bp.InitChunkBatch(recCtx, recordID, chunks, docCtx); err != nil {
			if s.Logger != nil {
				s.Logger.Error("chunk batch: init failed",
					"record_id", recordID,
					"processor", bp.Name(),
					"error", err,
				)
			}
			if batchFirstErr == nil {
				batchFirstErr = fmt.Errorf("(MID_26062707) %s init: %w", bp.Name(), err)
			}
		}
	}

	if s.Logger != nil {
		s.Logger.Info("chunk batch: scheduling three-phase cache-optimized run",
			"record_id", recordID,
			"num_processors", len(batchProcessors),
			"num_chunks", len(chunks),
		)
	}
	if schedErr := s.scheduleChunkBatch(ctx, batchProcessors, chunks, recordID); schedErr != nil {
		if errors.Is(schedErr, ErrPipelineStopped) {
			*requestStopped = true
			phaseBSpan.SetStatus(codes.Error, "stopped")
			return
		}
		if batchFirstErr == nil {
			batchFirstErr = schedErr
		}
	}

	// --- Finalize each processor ---
	for _, bp := range batchProcessors {
		if isCtxStopped(ctx) {
			*requestStopped = true
			phaseBSpan.SetStatus(codes.Error, "stopped")
			return
		}
		recCtx := withLLMRecordID(ctx, recordID)
		if err := bp.FinalizeChunkBatch(recCtx); err != nil {
			if errors.Is(err, ErrPipelineStopped) {
				*requestStopped = true
				return
			}
			if s.Logger != nil {
				s.Logger.Error("chunk batch: finalize failed",
					"record_id", recordID,
					"processor", bp.Name(),
					"error", err,
				)
			}
			if batchFirstErr == nil {
				batchFirstErr = fmt.Errorf("(MID_26062709) %s finalize: %w", bp.Name(), err)
			}
		}
	}

	if batchFirstErr != nil {
		*requestFailed = true
		*firstErr = batchFirstErr
		phaseBSpan.SetStatus(codes.Error, batchFirstErr.Error())
	} else {
		phaseBSpan.SetStatus(codes.Ok, "")
	}
}

// scheduleChunkBatch runs the four-phase DeepSeek cache schedule over already
// initialised batch processors: (1a) fire the seed processor's first chunk to
// prime the prompt-level cache, (1b) wait LLM_CALL_STAGGER, (1c) fire the
// seed's remaining chunks concurrently (prompt prefix now cached), (2) wait
// LLM_CALL_STAGGER for chunk+prompt prefixes to persist, (3) fire all remaining
// (processor x chunk) calls concurrently, bounded by MAX_DOC_PROCESSOR_TASKS.
// Mirrors the doc-reviewers' runReviewTasksForPromptCache.
// Returns the first ProcessChunk error, or ErrPipelineStopped on cancel.
func (s *ControlService) scheduleChunkBatch(
	ctx context.Context,
	batchProcessors []ChunkBatchProcessor,
	chunks []Chunk,
	recordID int64,
) error {
	if len(batchProcessors) == 0 || len(chunks) == 0 {
		return nil
	}
	ordered := orderBatchProcessorsSeedFirst(batchProcessors)
	seed := ordered[0]

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	record := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}
	runOne := func(bp ChunkBatchProcessor, chunkIdx int) {
		if isCtxStopped(ctx) {
			record(ErrPipelineStopped)
			return
		}
		recCtx := withLLMRecordID(ctx, recordID)
		if err := bp.ProcessChunk(recCtx, chunkIdx); err != nil {
			if errors.Is(err, ErrPipelineStopped) {
				record(ErrPipelineStopped)
				return
			}
			record(fmt.Errorf("(MID_26062708) %s chunk %d: %w", bp.Name(), chunkIdx, err))
		}
	}

	// Phase 1a: seed the first chunk only to prime the prompt-level cache.
	wg.Add(1)
	go func() { defer wg.Done(); runOne(seed, 0) }()

	// Phase 1b: wait for the prompt prefix to be cached. Skipped when there
	// are no subsequent LLM calls (single chunk, single processor).
	if len(chunks) > 1 || len(ordered) > 1 {
		if stagger := llmCallStagger(); stagger > 0 {
			select {
			case <-time.After(stagger):
			case <-ctx.Done():
				wg.Wait()
				return ErrPipelineStopped
			}
		}
	}

	// Phase 1c: seed remaining chunks concurrently; prompt prefix is now cached.
	for chunkIdx := 1; chunkIdx < len(chunks); chunkIdx++ {
		wg.Add(1)
		go func(idx int) { defer wg.Done(); runOne(seed, idx) }(chunkIdx)
	}

	// Phase 2: stagger while the seed keeps running.
	if len(ordered) > 1 {
		if stagger := llmCallStagger(); stagger > 0 {
			select {
			case <-time.After(stagger):
			case <-ctx.Done():
				wg.Wait()
				return ErrPipelineStopped
			}
		}
	}

	// Phase 3: remaining (processor x chunk) concurrently, bounded.
	// Chunk-major spawn order (chunks outer, processors inner) ensures
	// goroutines from different processors interleave. A processor-major
	// order would let the first processor's goroutines consume all
	// semaphore slots, serializing the effective execution per-processor
	// and evicting cached chunk prefixes before later processors use them.
	if len(ordered) > 1 {
		sem := make(chan struct{}, maxDocProcessorTasks(10))
		for chunkIdx := 0; chunkIdx < len(chunks); chunkIdx++ {
			for pi := 1; pi < len(ordered); pi++ {
				wg.Add(1)
				go func(bp ChunkBatchProcessor, idx int) {
					defer wg.Done()
					select {
					case sem <- struct{}{}:
					case <-ctx.Done():
						record(ErrPipelineStopped)
						return
					}
					defer func() { <-sem }()
					runOne(bp, idx)
				}(ordered[pi], chunkIdx)
			}
		}
	}

	wg.Wait()
	return firstErr
}

// runPhaseBProcessors selects the Phase B execution strategy:
// per-chunk batching when all processors support it, legacy concurrent mode otherwise.
func (s *ControlService) runPhaseBProcessors(
	ctx context.Context,
	payload []byte,
	processors []Processor,
	recordID int64,
	requestFailed, requestStopped *bool,
	firstErr *error,
	summaries *[]procResult,
) {
	phaseA, phaseB := splitProcessorsByPhase(processors)
	part := partitionBatchProcessors(phaseB)

	// Only one (or zero) batch-capable processor: batching yields no
	// cross-processor cache benefit, so run everything legacy.
	if len(part.batch) <= 1 {
		s.runProcessorsTwoPhase(ctx, payload, processors, recordID,
			requestFailed, requestStopped, firstErr, summaries)
		return
	}

	phaseACtx, phaseASpan := startPhaseSpan(ctx, "A", recordID, phaseA)
	for _, p := range phaseA {
		if p == nil {
			continue
		}
		if isCtxStopped(ctx) {
			*requestStopped = true
			phaseASpan.End()
			return
		}
		s.runSingleProcessor(phaseACtx, payload, p, recordID, requestFailed, firstErr, summaries)
		if !*requestFailed && canonicalOperationName(p.Name()) == "static_analyzer" {
			clearBlockBufferInContext(ctx)
		}
		if *requestFailed && isCtxStopped(ctx) {
			*requestFailed = false
			*firstErr = nil
			*requestStopped = true
			phaseASpan.End()
			return
		}
	}
	phaseASpan.End()
	if isCtxStopped(ctx) {
		*requestStopped = true
		return
	}

	if s.Logger != nil {
		s.Logger.Info("using per-chunk batch coordinator",
			"record_id", recordID,
		)
	}

	var (
		wg              sync.WaitGroup
		legacyFailed    bool
		legacyStopped   bool
		legacyErr       error
		legacySummaries []procResult
	)

	// Unsupported processors run legacy-concurrent alongside the batch path.
	if len(part.unsupported) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runProcessorsPhaseBOnly(ctx, payload, part.unsupported, recordID,
				&legacyFailed, &legacyStopped, &legacyErr, &legacySummaries)
		}()
	}

	s.runProcessorsChunkBatched(ctx, payload, phaseB, recordID,
		requestFailed, requestStopped, firstErr, summaries)

	wg.Wait()

	if legacyFailed {
		*requestFailed = true
		if *firstErr == nil {
			*firstErr = legacyErr
		}
	}
	if legacyStopped {
		*requestStopped = true
	}
	if summaries != nil {
		*summaries = append(*summaries, legacySummaries...)
	}
}

// runProcessorsPhaseBOnly fans out the given processors concurrently (no Phase A),
// used to run batch-unsupported processors alongside the chunk-batch coordinator.
func (s *ControlService) runProcessorsPhaseBOnly(
	ctx context.Context, payload []byte, phaseB []Processor,
	recordID int64, requestFailed, requestStopped *bool, firstErr *error,
	summaries *[]procResult,
) {
	if len(phaseB) == 0 {
		return
	}
	results := make([]procResult, len(phaseB))
	var wg sync.WaitGroup
	for i, p := range phaseB {
		wg.Add(1)
		go func(i int, p Processor) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[i] = procResult{failed: true, err: fmt.Errorf("(MID_26062740) processor %q panicked: %v", p.Name(), r)}
				}
			}()
			results[i] = s.runSingleProcessorCollect(ctx, payload, p, recordID)
		}(i, p)
	}
	wg.Wait()
	if summaries != nil {
		*summaries = append(*summaries, results...)
	}
	for _, r := range results {
		if r.failed {
			*requestFailed = true
			if *firstErr == nil {
				*firstErr = r.err
			}
		}
		if r.stopped {
			*requestStopped = true
		}
	}
}

// loadChunksFromBufferOrArtifact reads chunks from the context buffer if
// available, or loads from the artifact file.
func loadChunksFromBufferOrArtifact(
	ctx context.Context,
	rec DocMetadataInputRecord,
	lines []Line,
	recordID int64,
) []Chunk {
	if buf := ChunkBufferFromContext(ctx); buf != nil && len(buf.Chunks) > 0 {
		return buf.Chunks
	}
	artifactBase := buildChunkArtifactBaseName(rec.StagingFilename, rec.ParserName)
	artDir := strings.TrimSpace(os.Getenv("ARTIFACT_DIR"))
	chunks, err := loadChunksFromArtifactFile(artDir, recordID, artifactBase+".chunks", lines)
	if err != nil {
		return nil
	}
	return chunks
}

// recordError records an error on an OpenTelemetry span.
func recordError(span trace.Span, err error) {
	if span != nil && err != nil {
		span.SetAttributes(attribute.String("error.message", err.Error()))
		span.SetStatus(codes.Error, err.Error())
	}
}
