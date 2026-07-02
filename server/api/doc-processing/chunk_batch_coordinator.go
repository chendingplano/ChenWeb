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

// errChunkBatchUnsupported is returned when a processor does not implement
// ChunkBatchProcessor and the coordinator falls through to the legacy path.
var errChunkBatchUnsupported = errors.New("processor does not support per-chunk batching")

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
	summaries *[]procResult,
) {
	phaseB := make([]Processor, 0, len(processors))
	for _, p := range processors {
		if p != nil && !isPhaseAProcessor(p.Name()) {
			phaseB = append(phaseB, p)
		}
	}
	if len(phaseB) == 0 {
		return
	}

	// Verify all Phase B processors support batch mode.
	batchProcessors := make([]ChunkBatchProcessor, 0, len(phaseB))
	for _, p := range phaseB {
		bp, ok := p.(ChunkBatchProcessor)
		if !ok {
			// Fall back to legacy concurrent mode when any processor doesn't support batching.
			if s.Logger != nil {
				s.Logger.Info("chunk batch: processor does not support batching, falling back to legacy mode",
					"processor", p.Name(),
					"record_id", recordID,
				)
			}
			s.runProcessorsTwoPhase(ctx, payload, processors, recordID,
				requestFailed, requestStopped, firstErr, summaries)
			return
		}
		batchProcessors = append(batchProcessors, bp)
	}

	_, phaseBSpan := startPhaseSpan(ctx, "B", recordID, phaseB)
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

// scheduleChunkBatch runs the three-phase DeepSeek cache schedule over already
// initialised batch processors: (1) fire the seed processor's chunks
// concurrently, (2) wait LLM_CALL_STAGGER for the prefixes to persist, (3) fire
// all remaining (processor x chunk) calls concurrently, bounded by
// MAX_DOC_PROCESSOR_TASKS. Mirrors the doc-reviewers' runReviewTasksForPromptCache.
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

	// Phase 1: seed chunks concurrently; do NOT wait.
	for chunkIdx := 0; chunkIdx < len(chunks); chunkIdx++ {
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
	if len(ordered) > 1 {
		sem := make(chan struct{}, maxDocProcessorTasks(10))
		for pi := 1; pi < len(ordered); pi++ {
			for chunkIdx := 0; chunkIdx < len(chunks); chunkIdx++ {
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
	// Check if ALL Phase B processors implement ChunkBatchProcessor.
	allBatch := true
	for _, p := range processors {
		if p == nil || isPhaseAProcessor(p.Name()) {
			continue
		}
		if _, ok := p.(ChunkBatchProcessor); !ok {
			allBatch = false
			break
		}
	}

	if allBatch && len(processors) > 1 {
		if s.Logger != nil {
			s.Logger.Info("using per-chunk batch coordinator",
				"record_id", recordID,
			)
		}
		s.runProcessorsChunkBatched(ctx, payload, processors, recordID,
			requestFailed, requestStopped, firstErr, summaries)
	} else {
		if s.Logger != nil && allBatch {
			s.Logger.Info("only one processor, skipping per-chunk batching",
				"record_id", recordID)
		}
		s.runProcessorsTwoPhase(ctx, payload, processors, recordID,
			requestFailed, requestStopped, firstErr, summaries)
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
