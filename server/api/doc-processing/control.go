package docprocessing

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type Processor interface {
	Name() string
	HandleEvent(ctx context.Context, payload []byte) error
}

type logNamedProcessor interface {
	LogName() string
}

type ControlService struct {
	Processors        []Processor
	BlockingProcessor Processor // always runs before Processors, regardless of requested operations
	Logger            ApiTypes.JimoLogger
	InputStore        DocMetadataStore
	EventStore        EventStore
	StopStore         StopRequestStore
	Now               func() time.Time

	DocProcessorMode       string
	MaxDocProcessPipelines int
	pipelineSlots          chan struct{}
	pipelineSlotsMax       int
	pipelineSlotsMu        sync.Mutex
}

func (s *ControlService) HandleEvent(ctx context.Context, payload []byte) {
	s.handleEvent(ctx, payload)
}

func (s *ControlService) HandleJetStreamEvent(ctx context.Context, subject string, payload []byte) error {
	eventID := ""
	if s.EventStore != nil {
		id, err := s.insertReceivedEvent(ctx, subject, payload)
		if err != nil {
			if s.Logger != nil {
				s.Logger.Error("failed inserting kb.events received record", "subject", subject, "error", err)
			}
		} else {
			eventID = id
		}
	}

	releaseSlot, err := s.acquirePipelineSlot(ctx)
	if err != nil {
		return err
	}
	go func() {
		defer releaseSlot()
		procStart := s.now()
		var procErr error
		procCtx := withEventID(ctx, eventID)
		if strings.TrimSpace(subject) == DefaultEventSubject {
			procErr = s.handleDefaultSubjectEvent(procCtx, payload)
		} else {
			var spanEnd func(error)
			if evt, err := ParseLineFileGeneratedEvent(payload); err == nil {
				procCtx, spanEnd = s.startPipelineTrace(procCtx, subject, eventID, evt)
				defer func() { spanEnd(procErr) }()
			}
			procErr = s.handleEvent(procCtx, payload)
		}
		if s.EventStore == nil || strings.TrimSpace(eventID) == "" {
			return
		}
		if err := s.EventStore.UpsertConsumedStatus(context.Background(), eventID, procStart, time.Since(procStart).Milliseconds(), procErr); err != nil && s.Logger != nil {
			s.Logger.Error("failed upserting kb.events consumed status", "event_id", eventID, "error", err)
		}
	}()
	return nil
}

const (
	DocProcessorModeAuto = "auto"
	DocProcessorModeDev  = "dev"
)

func DocProcessorModeFromEnv() (string, error) {
	return normalizeDocProcessorMode(os.Getenv("DOC_PROCESSOR_MODE"))
}

func normalizeDocProcessorMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return DocProcessorModeAuto, nil
	}
	switch mode {
	case DocProcessorModeAuto, DocProcessorModeDev:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid DOC_PROCESSOR_MODE %q; allowed values are %q and %q", raw, DocProcessorModeAuto, DocProcessorModeDev)
	}
}

func (s *ControlService) docProcessorMode() (string, error) {
	if strings.TrimSpace(s.DocProcessorMode) != "" {
		return normalizeDocProcessorMode(s.DocProcessorMode)
	}
	return DocProcessorModeFromEnv()
}

func (s *ControlService) handleDefaultSubjectEvent(ctx context.Context, payload []byte) error {
	mode, err := s.docProcessorMode()
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("invalid doc processor mode", "error", err)
		}
		return err
	}
	switch mode {
	case DocProcessorModeAuto:
		return s.handleEvent(ctx, payload)
	case DocProcessorModeDev:
		return s.HandleStartDocProcessingEvent(ctx, payload)
	default:
		return fmt.Errorf("invalid doc processor mode %q", mode)
	}
}

type parsedInputRecordLister interface {
	ListParsedInputRecords(ctx context.Context) ([]DocMetadataInputRecord, error)
}

type failedDocProcessorRecordLister interface {
	ListRecordsWithFailedDocProcessors(ctx context.Context) ([]DocMetadataInputRecord, error)
}

func (s *ControlService) HandleStartDocProcessingEvent(ctx context.Context, payload []byte) error {
	cmd, err := ParseStartDocProcessingEvent(payload)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("doc processor failed parsing start command", "error", err)
		}
		return err
	}
	records, err := s.resolveStartDocProcessingRecords(ctx, cmd)
	if err != nil {
		return err
	}
	var (
		firstErr error
		mu       sync.Mutex
		wg       sync.WaitGroup
	)

	// Use a detached context so goroutines waiting for a pipeline slot
	// don't time out when the parent context has a short deadline.
	slotCtx := context.WithoutCancel(ctx)

	for _, rec := range records {
		ops := append([]string(nil), cmd.DocProcessors...)
		if len(ops) == 0 && cmd.FailedProcOnly {
			ops = failedDocProcessorNames(rec.StatusRaw)
		} else if cmd.FailedProcOnly {
			ops = intersectProcessorNames(ops, failedDocProcessorNames(rec.StatusRaw))
		}
		if cmd.FailedProcOnly && len(ops) == 0 {
			if s.Logger != nil {
				s.Logger.Info("doc processor skipped record, no failed processors selected",
					"record_id", rec.ID,
					"requested_processors", cmd.DocProcessors,
				)
			}
			continue
		}
		eventPayload, err := buildLineFileGeneratedPayload(rec.ID, cmd.Filename, ops)
		if err != nil {
			mu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			mu.Unlock()
			continue
		}

		// Launch each document's pipeline in its own goroutine, limited by
		// MaxDocProcessPipelines via the semaphore returned by acquirePipelineSlot.
		wg.Add(1)
		go func() {
			defer wg.Done()

			releaseSlot, slotErr := s.acquirePipelineSlot(slotCtx)
			if slotErr != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = slotErr
				}
				mu.Unlock()
				return
			}
			defer releaseSlot()

			if err := s.handleEvent(ctx, eventPayload); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return firstErr
}

func (s *ControlService) resolveStartDocProcessingRecords(ctx context.Context, cmd StartDocProcessingEvent) ([]DocMetadataInputRecord, error) {
	switch cmd.All {
	case "parsed":
		lister, ok := s.InputStore.(parsedInputRecordLister)
		if !ok {
			return nil, errors.New("input store does not support listing parsed records")
		}
		return lister.ListParsedInputRecords(ctx)
	case "failed-procs", "with-failed-procs":
		lister, ok := s.InputStore.(failedDocProcessorRecordLister)
		if !ok {
			return nil, errors.New("input store does not support listing records with failed doc processors")
		}
		return lister.ListRecordsWithFailedDocProcessors(ctx)
	}
	records := make([]DocMetadataInputRecord, 0, len(cmd.RecordIDs))
	for _, id := range cmd.RecordIDs {
		rec := DocMetadataInputRecord{ID: id}
		if s.InputStore != nil {
			loaded, err := s.InputStore.GetInputRecord(ctx, id)
			if err != nil {
				return nil, fmt.Errorf("load kb.inputs record %d: %w", id, err)
			}
			rec = loaded
		}
		records = append(records, rec)
	}
	return records, nil
}

func buildLineFileGeneratedPayload(recordID int64, filename string, operations []string) ([]byte, error) {
	body := map[string]any{"record_id": strconv.FormatInt(recordID, 10)}
	if strings.TrimSpace(filename) != "" {
		body["filename"] = strings.TrimSpace(filename)
	}
	if len(operations) > 0 {
		body["operation"] = operations
	}
	return json.Marshal(body)
}

func intersectProcessorNames(requested []string, failed []string) []string {
	failedSet := make(map[string]struct{}, len(failed))
	for _, name := range failed {
		if key := canonicalOperationName(name); key != "" {
			failedSet[key] = struct{}{}
		}
	}
	out := make([]string, 0, len(requested))
	seen := map[string]struct{}{}
	for _, name := range requested {
		key := canonicalOperationName(name)
		if key == "" {
			continue
		}
		if _, ok := failedSet[key]; !ok {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

func failedDocProcessorNames(statusRaw string) []string {
	entries := decodeDocMetaStatus(statusRaw)
	out := make([]string, 0, len(entries))
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if statusValue(entry) != "failed" {
			continue
		}
		op := canonicalOperationName(asString(entry["operation"]))
		if !isDocProcessorStatusOperation(op) {
			continue
		}
		if _, ok := seen[op]; ok {
			continue
		}
		seen[op] = struct{}{}
		out = append(out, op)
	}
	return out
}

func (s *ControlService) resetProcessorStatuses(ctx context.Context, recordID int64, processors []Processor) {
	if s.InputStore == nil || recordID <= 0 || len(processors) == 0 {
		return
	}
	names := make([]string, 0, len(processors))
	for _, p := range processors {
		if p == nil {
			continue
		}
		if name := canonicalOperationName(p.Name()); isDocProcessorStatusOperation(name) {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return
	}
	if err := updateInputStatusAtomic(ctx, s.InputStore, recordID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := resetRequestedProcessorStatuses(current, names)
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		return DocMetadataUpdate{StatusRaw: statusRaw}, nil
	}); err != nil && s.Logger != nil {
		s.Logger.Error("failed resetting selected doc processor statuses", "record_id", recordID, "processors", names, "error", err)
	}
}

func resetRequestedProcessorStatuses(raw string, requested []string) (string, error) {
	targets := make(map[string]struct{}, len(requested))
	for _, name := range requested {
		for _, alias := range processorStatusAliases(name) {
			targets[alias] = struct{}{}
		}
	}
	if len(targets) == 0 {
		if strings.TrimSpace(raw) == "" {
			return "[]", nil
		}
		return raw, nil
	}

	entries := decodeDocMetaStatus(raw)
	out := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		op := canonicalOperationName(asString(entry["operation"]))
		if _, ok := targets[op]; ok {
			continue
		}
		out = append(out, entry)
	}

	bs, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func processorStatusAliases(name string) []string {
	key := canonicalOperationName(name)
	switch key {
	case "chunking":
		return []string{"chunking", "chunked"}
	case "extract_doc_metadata":
		return []string{"extract_doc_metadata", "extract_metadata"}
	case "generate_scene_blocks":
		return []string{"generate_scene_blocks", "extract_scene_blocks"}
	case "extract_semantic_projections":
		return []string{"extract_semantic_projections", "extract_semantic_projection"}
	case "extract_entity":
		return []string{"extract_entity", "extract_entity_relation"}
	case "extract_relation":
		return []string{"extract_relation", "extract_entity_relation"}
	case "extract_inventory_items":
		return []string{"extract_inventory_items"}
	case "extract_structured_knowledge":
		return []string{"extract_structured_knowledge", "extract_structured_knowledges"}
	default:
		if key == "" {
			return nil
		}
		return []string{key}
	}
}

func statusValue(entry map[string]any) string {
	return strings.ToLower(strings.TrimSpace(firstNonEmptyTrimmed(
		asString(entry["proc_status"]),
		asString(entry["status"]),
	)))
}

func isDocProcessorStatusOperation(op string) bool {
	switch canonicalOperationName(op) {
	case "", "parsed", "parsing", "converted", "converting", "doc_processing", "doc_processor":
		return false
	default:
		return true
	}
}

func (s *ControlService) startPipelineTrace(ctx context.Context, subject string, eventID string, evt LineFileGeneratedEvent) (context.Context, func(error)) {
	ctx, span := startPipelineSpan(ctx, subject, eventID, evt)
	return ctx, func(err error) {
		status := "success"
		if err != nil {
			status = "failed"
		}
		span.SetAttributes(attribute.String("pipeline.status", status))
		endSpanWithStatus(span, status, err)
		span.End()
	}
}

func (s *ControlService) acquirePipelineSlot(ctx context.Context) (func(), error) {
	limit := s.maxDocProcessPipelines()
	if limit <= 0 {
		return func() {}, nil
	}
	s.pipelineSlotsMu.Lock()
	if s.pipelineSlots == nil || s.pipelineSlotsMax != limit {
		s.pipelineSlots = make(chan struct{}, limit)
		s.pipelineSlotsMax = limit
	}
	slots := s.pipelineSlots
	s.pipelineSlotsMu.Unlock()

	select {
	case slots <- struct{}{}:
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *ControlService) maxDocProcessPipelines() int {
	if s.MaxDocProcessPipelines > 0 {
		return s.MaxDocProcessPipelines
	}
	return MaxDocProcessPipelinesFromEnv()
}

// RunDocProcessorConcurrentFromEnv reports whether Phase B processors run
// concurrently. Defaults to true; set RUN_DOC_PROCESSOR_CONCURRENT=false to
// fall back to the sequential pipeline.
func RunDocProcessorConcurrentFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RUN_DOC_PROCESSOR_CONCURRENT")))
	return v != "false"
}

func MaxDocProcessPipelinesFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("MAX_DOC_PROCESS_PIPELINES"))
	if raw == "" {
		return 10
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 10
	}
	return n
}

func (s *ControlService) handleEvent(ctx context.Context, payload []byte) error {
	requestStart := s.now()
	processors := s.Processors
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("doc processor failed parsing event", "error", err)
			s.Logger.Info("finish processing request",
				"record_id", int64(0),
				"proc_status", "failed",
				"ms_used", time.Since(requestStart).Milliseconds(),
			)
		}
		return err
	}
	if s.Logger != nil {
		s.Logger.Info("start processing request",
			"record_id", evt.RecordID,
			"operations", evt.Operations,
		)
	}

	if skipReason := skipReasonLineFileGeneratedEvent(evt); skipReason != "" {
		if s.Logger != nil {
			s.Logger.Info("doc processor skipped event",
				"record_id", evt.RecordID,
				"reason", skipReason,
				"type", evt.Type,
				"status", evt.Status,
				"filename", evt.Filename,
				"operations", evt.Operations,
			)
			s.Logger.Info("finish processing request",
				"record_id", evt.RecordID,
				"proc_status", "skipped",
				"ms_used", time.Since(requestStart).Milliseconds(),
			)
		}
		return nil
	}
	if !s.preflightInput(ctx, evt) {
		if s.Logger != nil {
			s.Logger.Info("finish processing request",
				"record_id", evt.RecordID,
				"proc_status", "failed",
				"ms_used", time.Since(requestStart).Milliseconds(),
			)
		}
		return errors.New("(MID_26042404) preflight input failed")
	}
	if len(evt.Operations) > 0 {
		processors = s.selectProcessors(evt.Operations)
		processors = s.skipSatisfiedAutoDependencies(ctx, evt, processors)
		if len(processors) == 0 {
			if s.Logger != nil {
				allowed := make([]string, 0, len(s.Processors))
				for _, p := range s.Processors {
					allowed = append(allowed, p.Name())
				}
				s.Logger.Info("doc processor skipped, no matching operation", "requested", evt.Operations, "allowed", allowed)
				s.Logger.Info("finish processing request",
					"record_id", evt.RecordID,
					"proc_status", "skipped",
					"ms_used", time.Since(requestStart).Milliseconds(),
				)
			}
			return nil
		}
	}
	if len(processors) > 0 {
		s.resetProcessorStatuses(ctx, evt.RecordID, processors)
	}
	// Inject buffer holders so blocking and chunking processors can share
	// their output with downstream processors via context.
	ctx, _ = withBlockBufferHolder(ctx)
	ctx, _ = withChunkBufferHolder(ctx)

	// Clear any stale stop_requested flag left over from a previous run that was
	// killed before the deferred ClearStopRequested could execute. Without this,
	// pollForStop would cancel the new context within 1 s of startup.
	if s.StopStore != nil {
		if err := s.StopStore.ClearStopRequested(ctx, evt.RecordID); err != nil && s.Logger != nil {
			s.Logger.Warn("failed to clear stale stop_requested flag at pipeline start", "record_id", evt.RecordID, "error", err)
		}
	}

	// Wrap with a cancellable context so a user stop request can interrupt
	// in-flight LLM calls. pollForStop cancels the context (with cause
	// ErrPipelineStopped) when it detects the stop flag in the database.
	if s.StopStore != nil {
		var cancelCause context.CancelCauseFunc
		ctx, cancelCause = context.WithCancelCause(ctx)
		defer cancelCause(nil) // ensures polling goroutine exits when pipeline completes
		go s.pollForStop(ctx, evt.RecordID, cancelCause)
	}

	requestFailed := false
	requestStopped := false
	var firstErr error
	s.persistPipelineStatus(ctx, evt.RecordID, "running", "", nil)
	defer func() {
		status := "success"
		if requestStopped {
			status = "stopped"
		} else if requestFailed {
			status = "failed"
		}
		s.persistPipelineStatus(context.Background(), evt.RecordID, status, "", firstErr)
		if requestStopped && s.StopStore != nil {
			_ = s.StopStore.ClearStopRequested(context.Background(), evt.RecordID)
		}
	}()

	var allProcResults []procResult

	// Always run the blocking processor first, regardless of requested operations.
	if s.BlockingProcessor != nil {
		s.runSingleProcessor(ctx, payload, s.BlockingProcessor, evt.RecordID, &requestFailed, &firstErr, &allProcResults)
	}

	if RunDocProcessorConcurrentFromEnv() {
		s.runPhaseBProcessors(ctx, payload, processors, evt.RecordID, &requestFailed, &requestStopped, &firstErr, &allProcResults)
	} else {
		s.runProcessorsSequential(ctx, payload, processors, evt.RecordID, &requestFailed, &requestStopped, &firstErr, &allProcResults)
	}
	if requestStopped {
		return nil
	}

	// Phase C (post-process): now that every doc processor has finished, index the
	// artifacts of the invoked processors that defer indexing to this phase. This is the
	// only place cross-artifact indexing (e.g. metrics) may run, so it sees all outputs.
	s.runPostProcessIndexing(ctx, processors, evt.RecordID)

	pipelineMSUsed := time.Since(requestStart).Milliseconds()
	status := "success"
	if requestFailed {
		status = "failed"
	}
	if s.Logger != nil {
		s.Logger.Info("finish processing request",
			"record_id", evt.RecordID,
			"proc_status", status,
			"ms_used", pipelineMSUsed,
		)
	}
	s.logPipelineFinish(context.Background(), evt.RecordID, pipelineMSUsed, allProcResults)
	return firstErr
}

func (s *ControlService) skipSatisfiedAutoDependencies(ctx context.Context, evt LineFileGeneratedEvent, processors []Processor) []Processor {
	if len(processors) == 0 || !requestedOperationsNeedAutoChunking(evt.Operations) || s.InputStore == nil {
		return processors
	}
	rec, err := s.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil || !docProcessorSucceeded(rec.StatusRaw, "chunking") {
		return processors
	}
	explicit := make(map[string]struct{}, len(evt.Operations))
	for _, op := range evt.Operations {
		if key := canonicalOperationName(op); key != "" {
			explicit[key] = struct{}{}
		}
	}
	pruned := make([]Processor, 0, len(processors))
	for _, p := range processors {
		name := canonicalOperationName(p.Name())
		if _, ok := explicit[name]; ok {
			pruned = append(pruned, p)
			continue
		}
		if name == "static_analyzer" || name == "chunking" {
			continue
		}
		pruned = append(pruned, p)
	}
	return pruned
}

// PostProcessIndexer is implemented by processors that index their artifacts in
// Phase C (post-process) rather than inside HandleEvent. Phase C runs once, after every
// doc processor in the pipeline has finished, so cross-artifact indexing (for example
// metrics, which links to semantic_projections, topics, scenes, provisions, entities and
// inventory items) can see every processor's output regardless of Phase B ordering.
// Implementations must be idempotent.
type PostProcessIndexer interface {
	PostProcessIndex(ctx context.Context, recordID int64) error
}

// runPostProcessIndexing executes Phase C: for each invoked processor that implements
// PostProcessIndexer, run its indexing step concurrently (one goroutine per processor).
// Errors are logged and do not abort sibling processors' indexing. The caller skips this
// when the pipeline was stopped.
func (s *ControlService) runPostProcessIndexing(ctx context.Context, processors []Processor, recordID int64) {
	if isCtxStopped(ctx) {
		return
	}
	ctx, phaseSpan := startPhaseSpan(ctx, "C", recordID, processors)
	defer phaseSpan.End()

	var wg sync.WaitGroup
	for _, p := range processors {
		indexer, ok := p.(PostProcessIndexer)
		if !ok {
			continue
		}
		wg.Add(1)
		go func(p Processor, indexer PostProcessIndexer) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					if s.Logger != nil {
						s.Logger.Error("post-process indexing panicked", "record_id", recordID, "processor", processorLogName(p), "panic", r)
					}
				}
			}()

			startTime := time.Now()
			name := processorLogName(p)
			if s.Logger != nil {
				s.Logger.Info("post-process indexing start", "record_id", recordID, "processor", name)
			}
			indexCtx, indexSpan := otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.post_process_index",
				trace.WithAttributes(
					attribute.Int64("doc.record_id", recordID),
					attribute.String("processor.name", name),
					attribute.String("processor.phase", "C"),
				),
			)
			if err := indexer.PostProcessIndex(indexCtx, recordID); err != nil {
				indexSpan.RecordError(err)
				indexSpan.SetStatus(codes.Error, err.Error())
				indexSpan.SetAttributes(attribute.String("index.status", "failed"))
				indexSpan.End()
				if s.Logger != nil {
					s.Logger.Error("post-process indexing failed", "record_id", recordID, "processor", name, "error", err)
				}
				return
			}
			indexSpan.SetAttributes(attribute.String("index.status", "success"))
			indexSpan.SetStatus(codes.Ok, "success")
			indexSpan.End()
			if s.Logger != nil {
				s.Logger.Info("post-process indexing finished",
					"record_id", recordID,
					"processor", name,
					"ms_used", time.Since(startTime).Milliseconds(),
				)
			}
		}(p, indexer)
	}
	wg.Wait()
}

// isPhaseAProcessor reports whether the named processor is a mandatory,
// sequential Phase A processor. Must stay in sync with the mandatory set in
// cmd/doc-processor/main.go filterConfiguredProcessors.
func isPhaseAProcessor(name string) bool {
	switch canonicalOperationName(name) {
	case "static_analyzer", "chunking", "extract_doc_metadata":
		return true
	default:
		return false
	}
}

type procResult struct {
	failed    bool
	stopped   bool
	err       error
	operation string
	msUsed    int64
}

// runSingleProcessorCollect runs one processor and returns its outcome without
// mutating shared controller state, so it is safe to call from concurrent
// goroutines. All status writes inside go through the per-record status lock.
func (s *ControlService) runSingleProcessorCollect(ctx context.Context, payload []byte, p Processor, recordID int64) procResult {
	procStart := s.now()
	processorName := processorLogName(p)
	ctx, span := startProcessorSpan(ctx, p, processorPhase(p), recordID)
	defer span.End()
	if s.Logger != nil {
		s.Logger.Info("start running processor",
			"record_id", recordID,
			"processor", processorName,
		)
	}
	s.persistPipelineStatus(ctx, recordID, "running", processorName, nil)
	if err := p.HandleEvent(ctx, payload); err != nil {
		res := procResult{failed: true, err: err, operation: processorName, msUsed: time.Since(procStart).Milliseconds()}
		procStatus := "failed"
		if errors.Is(err, ErrPipelineStopped) || isCtxStopped(ctx) {
			res.stopped = true
			procStatus = "stopped"
			if s.Logger != nil {
				s.Logger.Info("processor stopped by user request", "processor", processorName, "record_id", recordID)
			}
		} else if s.Logger != nil {
			s.Logger.Error("doc processor failed", "processor", processorName, "error", err)
		}
		if s.Logger != nil {
			s.Logger.Info("finish running processor",
				"record_id", recordID,
				"processor", processorName,
				"proc_status", procStatus,
				"ms_used", res.msUsed,
			)
		}
		setProcessorSpanResult(span, procStatus, err, res.msUsed)
		return res
	}
	res := procResult{operation: processorName, msUsed: time.Since(procStart).Milliseconds()}
	if s.Logger != nil {
		s.Logger.Info("finish running processor",
			"record_id", recordID,
			"processor", processorName,
			"proc_status", "success",
			"ms_used", res.msUsed,
		)
	}
	setProcessorSpanResult(span, "success", nil, res.msUsed)
	return res
}

func (s *ControlService) runSingleProcessor(ctx context.Context, payload []byte, p Processor, recordID int64, requestFailed *bool, firstErr *error, summaries *[]procResult) {
	res := s.runSingleProcessorCollect(ctx, payload, p, recordID)
	if summaries != nil {
		*summaries = append(*summaries, res)
	}
	if res.failed {
		*requestFailed = true
		if *firstErr == nil {
			*firstErr = res.err
		}
	}
}

// runProcessorsSequential runs all processors in order exactly as the original
// sequential loop did. Used as the fallback when RUN_DOC_PROCESSOR_CONCURRENT=false.
func (s *ControlService) runProcessorsSequential(
	ctx context.Context, payload []byte, processors []Processor,
	recordID int64, requestFailed, requestStopped *bool, firstErr *error,
	summaries *[]procResult,
) {
	phaseA, phaseB := splitProcessorsByPhase(processors)
	phaseACtx, phaseASpan := startPhaseSpan(ctx, "A", recordID, phaseA)
	phaseAEnded := false
	endPhaseA := func() {
		if !phaseAEnded {
			phaseASpan.End()
			phaseAEnded = true
		}
	}
	defer endPhaseA()
	var phaseBSpan trace.Span
	phaseBCtx := ctx
	phaseBStarted := false
	for _, p := range processors {
		if p == nil {
			continue
		}
		if isCtxStopped(ctx) {
			*requestStopped = true
			if s.Logger != nil {
				s.Logger.Info("doc processor stop requested, halting pipeline", "record_id", recordID)
			}
			return
		}
		runCtx := phaseACtx
		if !isPhaseAProcessor(p.Name()) {
			if !phaseBStarted {
				endPhaseA()
				phaseBCtx, phaseBSpan = startPhaseSpan(ctx, "B", recordID, phaseB)
				phaseBStarted = true
				defer phaseBSpan.End()
			}
			runCtx = phaseBCtx
		}
		s.runSingleProcessor(runCtx, payload, p, recordID, requestFailed, firstErr, summaries)
		if !*requestFailed && canonicalOperationName(p.Name()) == "static_analyzer" {
			clearBlockBufferInContext(ctx)
		}
		if *requestFailed && isCtxStopped(ctx) {
			*requestFailed = false
			*firstErr = nil
			*requestStopped = true
			if s.Logger != nil {
				s.Logger.Info("doc processor stop detected after processor", "record_id", recordID, "processor", p.Name())
			}
			return
		}
	}
}

// runProcessorsTwoPhase runs Phase A processors (mandatory, sequential) and then
// fans out Phase B (configurable, concurrent) under a WaitGroup. Each goroutine
// runs independently via runSingleProcessorCollect so shared controller state is
// not mutated. A panic in a Phase B processor is recovered and recorded as a
// failure without crashing the parent.
func (s *ControlService) runProcessorsTwoPhase(
	ctx context.Context, payload []byte, processors []Processor,
	recordID int64, requestFailed, requestStopped *bool, firstErr *error,
	summaries *[]procResult,
) {
	phaseA, phaseB := splitProcessorsByPhase(processors)
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
	if len(phaseB) == 0 {
		return
	}

	phaseBCtx, phaseBSpan := startPhaseSpan(ctx, "B", recordID, phaseB)
	defer phaseBSpan.End()
	results := make([]procResult, len(phaseB))
	var wg sync.WaitGroup
	for i, p := range phaseB {
		wg.Add(1)
		go func(i int, p Processor) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[i] = procResult{failed: true, err: fmt.Errorf("(MID_26060101) processor %q panicked: %v", p.Name(), r)}
					if s.Logger != nil {
						s.Logger.Error("doc processor panicked", "processor", p.Name(), "record_id", recordID, "panic", r)
					}
				}
			}()
			results[i] = s.runSingleProcessorCollect(phaseBCtx, payload, p, recordID)
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
	}
	if isCtxStopped(ctx) {
		*requestFailed = false
		*firstErr = nil
		*requestStopped = true
	}
}

type pipelineProcSummaryJSON struct {
	Operation  string `json:"operation"`
	ProcStatus string `json:"proc_status"`
	MSUsed     int64  `json:"ms_used"`
}

func (s *ControlService) logPipelineFinish(ctx context.Context, recordID int64, msUsed int64, results []procResult) {
	summaries := make([]pipelineProcSummaryJSON, len(results))
	for i, r := range results {
		procStatus := "success"
		if r.stopped {
			procStatus = "stopped"
		} else if r.failed {
			procStatus = "failed"
		}
		summaries[i] = pipelineProcSummaryJSON{
			Operation:  r.operation,
			ProcStatus: procStatus,
			MSUsed:     r.msUsed,
		}
	}
	extraInfoBytes, err := json.Marshal(summaries)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Warn("logPipelineFinish: failed marshalling extra_info", "record_id", recordID, "error", err)
		}
		return
	}
	extraInfoStr := string(extraInfoBytes)
	activityName := "pipeline finish"
	logger := DocProcLogger{}
	if err := logger.LogPipelineFinish(ctx, DocProcLogRecord{
		CallReason:    "pipeline finish",
		DocProcName:   "pipeline",
		RecordID:      int64Ptr(recordID),
		ActivityName:  &activityName,
		MSUsed:        int64Ptr(msUsed),
		ExtraInfoJSON: &extraInfoStr,
	}, "MID-26060801"); err != nil && s.Logger != nil {
		s.Logger.Warn("failed to write pipeline finish log", "record_id", recordID, "error", err)
	}
}

func (s *ControlService) persistPipelineStatus(ctx context.Context, recordID int64, procStatus string, processorName string, procErr error) {
	if s.InputStore == nil || recordID <= 0 {
		return
	}
	err := updateInputStatusAtomic(ctx, s.InputStore, recordID, func(current string) (DocMetadataUpdate, error) {
		statusRaw, err := appendPipelineStatus(current, s.now(), procStatus, processorName, procErr)
		if err != nil {
			return DocMetadataUpdate{}, err
		}
		var errMsg *string
		if procErr != nil {
			msg := strings.TrimSpace(procErr.Error())
			errMsg = &msg
		}
		return DocMetadataUpdate{StatusRaw: statusRaw, ErrorMsg: errMsg}, nil
	})
	if err != nil && s.Logger != nil {
		s.Logger.Error("failed persisting doc pipeline status", "record_id", recordID, "error", err)
	}
}

func processorLogName(p Processor) string {
	if p == nil {
		return ""
	}
	if named, ok := p.(logNamedProcessor); ok {
		if logName := canonicalOperationName(named.LogName()); logName != "" {
			return logName
		}
	}
	return p.Name()
}

func splitProcessorsByPhase(processors []Processor) ([]Processor, []Processor) {
	phaseA := make([]Processor, 0, len(processors))
	phaseB := make([]Processor, 0, len(processors))
	for _, p := range processors {
		if p == nil {
			continue
		}
		if isPhaseAProcessor(p.Name()) {
			phaseA = append(phaseA, p)
			continue
		}
		phaseB = append(phaseB, p)
	}
	return phaseA, phaseB
}

func (s *ControlService) preflightInput(ctx context.Context, evt LineFileGeneratedEvent) bool {
	if s.InputStore == nil {
		return true
	}
	_, recordSpan := otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.record.load",
		trace.WithAttributes(attribute.Int64("doc.record_id", evt.RecordID)),
	)
	rec, err := s.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		recordSpan.RecordError(err)
		recordSpan.SetStatus(codes.Error, err.Error())
		recordSpan.End()
		if errors.Is(err, sql.ErrNoRows) {
			if s.Logger != nil {
				s.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			}
			return false
		}
		if s.Logger != nil {
			s.Logger.Error("load kb.inputs record failed", "record_id", evt.RecordID, "error", err)
		}
		return false
	}
	recordSpan.SetAttributes(
		attribute.String("doc.parser_name", strings.TrimSpace(rec.ParserName)),
		attribute.String("doc.result_filename", strings.TrimSpace(rec.ResultFilename)),
	)
	recordSpan.SetStatus(codes.Ok, "success")
	recordSpan.End()

	if strings.TrimSpace(rec.ParserName) == "" {
		s.persistControlFailure(ctx, rec, errors.New("(MID_26042401) missing parser name"))
		return false
	}
	if strings.TrimSpace(rec.ResultFilename) == "" {
		s.persistControlFailure(ctx, rec, errors.New("(MID_26042402) missing result filename"))
		return false
	}

	_, resolveSpan := otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.input_file.resolve",
		trace.WithAttributes(
			attribute.Int64("doc.record_id", evt.RecordID),
			attribute.String("doc.parser_name", strings.TrimSpace(rec.ParserName)),
		),
	)
	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		resolveSpan.RecordError(err)
		resolveSpan.SetStatus(codes.Error, err.Error())
		resolveSpan.End()
		s.persistControlFailure(ctx, rec, err)
		return false
	}
	resolveSpan.SetAttributes(attribute.String("doc.input_path", inputPath))
	resolveSpan.SetStatus(codes.Ok, "success")
	resolveSpan.End()

	_, validateSpan := otel.Tracer(docProcessorTracerName).Start(ctx, "doc_processor.input_file.validate",
		trace.WithAttributes(
			attribute.Int64("doc.record_id", evt.RecordID),
			attribute.String("doc.input_path", inputPath),
		),
	)
	fi, err := os.Stat(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			procErr := fmt.Errorf("(MID_26042405) input file not exist: %s", inputPath)
			validateSpan.RecordError(procErr)
			validateSpan.SetStatus(codes.Error, procErr.Error())
			validateSpan.End()
			s.persistControlFailure(ctx, rec, procErr)
			return false
		}
		procErr := fmt.Errorf("(MID_26042406) stat input file: %w", err)
		validateSpan.RecordError(procErr)
		validateSpan.SetStatus(codes.Error, procErr.Error())
		validateSpan.End()
		s.persistControlFailure(ctx, rec, procErr)
		return false
	}
	if fi.Size() == 0 {
		procErr := errors.New("(MID_26042403) input file empty")
		validateSpan.RecordError(procErr)
		validateSpan.SetStatus(codes.Error, procErr.Error())
		validateSpan.End()
		s.persistControlFailure(ctx, rec, procErr)
		return false
	}
	validateSpan.SetAttributes(attribute.Int64("doc.input_size_bytes", fi.Size()))
	validateSpan.SetStatus(codes.Ok, "success")
	validateSpan.End()
	return true
}

func (s *ControlService) persistControlFailure(ctx context.Context, rec DocMetadataInputRecord, procErr error) {
	if s.InputStore == nil {
		return
	}
	errMsg := strings.TrimSpace(procErr.Error())
	statusRaw, err := appendControlStatus(rec.StatusRaw, s.now(), procErr)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("failed building doc processor failure status", "record_id", rec.ID, "error", err)
		}
		return
	}
	if err := s.InputStore.UpdateInputMetadata(ctx, rec.ID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  &errMsg,
	}); err != nil && s.Logger != nil {
		s.Logger.Error("failed persisting doc processor failure status", "record_id", rec.ID, "error", err)
	}
	if s.Logger != nil {
		s.Logger.Error("doc processor input validation failed", "record_id", rec.ID, "error", procErr)
	}
}

// pollForStop polls the database every second and cancels the pipeline context
// (with cause ErrPipelineStopped) if a stop request is detected. It exits when
// ctx is cancelled (either by the stop itself or by the pipeline completing).
func (s *ControlService) pollForStop(ctx context.Context, recordID int64, cancelCause context.CancelCauseFunc) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ok, _ := s.StopStore.IsStopRequested(context.Background(), recordID); ok {
				if s.Logger != nil {
					s.Logger.Info("stop request detected, cancelling pipeline context", "record_id", recordID)
				}
				cancelCause(ErrPipelineStopped)
				return
			}
		}
	}
}

func (s *ControlService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *ControlService) insertReceivedEvent(ctx context.Context, subject string, payload []byte) (string, error) {
	eventID := newEventID()
	if s.EventStore == nil {
		return eventID, nil
	}
	start := s.now()
	status := []map[string]any{
		{
			"operation":   "received",
			"proc_status": "success",
			"start_time":  start.Format(defaultDocMetaStatusTime),
			"ms_used":     int64(0),
		},
	}
	statusJSON, err := json.Marshal(status)
	if err != nil {
		return "", err
	}
	eventPayload := normalizeStoredEventPayload(payload)
	req := EventRecord{
		EventName:    DefaultEventSubject,
		EventID:      eventID,
		EventSubject: strings.TrimSpace(subject),
		EventPayload: eventPayload,
		EventStatus:  string(statusJSON),
		EventSource:  "JetStream",
		EventNotes:   "",
	}
	if err := s.EventStore.InsertEvent(ctx, req); err != nil {
		return "", err
	}
	return eventID, nil
}

func normalizeStoredEventPayload(payload []byte) string {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" {
		return "{}"
	}
	if json.Valid([]byte(trimmed)) {
		return trimmed
	}
	out, err := json.Marshal(map[string]any{
		"_raw_payload":   trimmed,
		"_payload_error": "invalid_json",
	})
	if err != nil {
		return `{"_payload_error":"invalid_json"}`
	}
	return string(out)
}

type eventIDCtxKey struct{}

func withEventID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, eventIDCtxKey{}, id)
}

func eventIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(eventIDCtxKey{}).(string)
	return id
}

func newEventID() string {
	var bs [12]byte
	if _, err := rand.Read(bs[:]); err != nil {
		return fmt.Sprintf("evt-%d", time.Now().UnixNano())
	}
	return "evt-" + hex.EncodeToString(bs[:])
}

func appendPipelineStatus(raw string, now time.Time, procStatus string, processorName string, procErr error) (string, error) {
	status := strings.ToLower(strings.TrimSpace(procStatus))
	if status == "" {
		status = "running"
	}
	entry := map[string]any{
		"operation":   "doc_processing",
		"start_time":  now.Format(defaultDocMetaStatusTime),
		"proc_status": status,
	}
	if processorName != "" {
		entry["doc_processor_name"] = processorName
	}
	if procErr != nil {
		entry["error"] = strings.TrimSpace(procErr.Error())
	}

	entries := decodeDocMetaStatus(raw)
	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "doc_processing" {
			out = append(out, e)
			continue
		}
		if !replaced {
			if originalStart := strings.TrimSpace(asString(e["start_time"])); originalStart != "" {
				entry["start_time"] = originalStart
			}
			out = append(out, entry)
			replaced = true
		}
	}
	if !replaced {
		out = append(out, entry)
	}

	bs, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func appendControlStatus(raw string, now time.Time, procErr error) (string, error) {
	entries := decodeDocMetaStatus(raw)
	entry := map[string]any{
		"operation":   "doc_processor",
		"start_time":  now.Format(defaultDocMetaStatusTime),
		"ms-used":     int64(0),
		"proc-status": "failed",
		"error":       strings.TrimSpace(procErr.Error()),
	}

	replaced := false
	out := make([]map[string]any, 0, len(entries)+1)
	for _, e := range entries {
		op := strings.ToLower(strings.TrimSpace(asString(e["operation"])))
		if op != "doc_processor" {
			out = append(out, e)
			continue
		}
		if !replaced {
			out = append(out, entry)
			replaced = true
		}
	}
	if !replaced {
		out = append(out, entry)
	}

	bs, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(bs), nil
}

func (s *ControlService) selectProcessors(ops []string) []Processor {
	if len(ops) == 0 {
		return s.Processors
	}

	available := make(map[string]Processor, len(s.Processors))
	for _, p := range s.Processors {
		if p == nil {
			continue
		}
		available[canonicalOperationName(p.Name())] = p
	}

	expanded := expandProcessorDependencies(ops)
	selected := make([]Processor, 0, len(expanded))
	seen := make(map[string]struct{}, len(expanded))
	for _, op := range expanded {
		key := canonicalOperationName(op)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		p, ok := available[key]
		if !ok {
			if s.Logger != nil {
				s.Logger.Info("doc processor operation ignored", "operation", op)
			}
			continue
		}
		seen[key] = struct{}{}
		selected = append(selected, p)
	}
	return selected
}

func requestedOperationsNeedAutoChunking(ops []string) bool {
	if len(ops) == 0 {
		return false
	}
	for _, raw := range ops {
		if canonicalOperationName(raw) == "chunking" {
			return false
		}
	}
	for _, raw := range ops {
		if requiresChunkingDependency(raw) {
			return true
		}
	}
	return false
}

func docProcessorSucceeded(statusRaw string, op string) bool {
	want := canonicalOperationName(op)
	if want == "" {
		return false
	}
	for _, entry := range decodeDocMetaStatus(statusRaw) {
		if canonicalOperationName(asString(entry["operation"])) != want {
			continue
		}
		if statusValue(entry) == "success" {
			return true
		}
	}
	return false
}

func expandProcessorDependencies(ops []string) []string {
	if len(ops) == 0 {
		return nil
	}
	expanded := make([]string, 0, len(ops)+2)
	appendOnce := func(op string) {
		key := canonicalOperationName(op)
		if key == "" {
			return
		}
		for _, existing := range expanded {
			if canonicalOperationName(existing) == key {
				return
			}
		}
		expanded = append(expanded, key)
	}

	hasRequestedChunking := false
	for _, raw := range ops {
		if canonicalOperationName(raw) == "chunking" {
			hasRequestedChunking = true
			break
		}
	}
	for _, raw := range ops {
		op := canonicalOperationName(raw)
		switch op {
		case "chunking":
			appendOnce("static_analyzer")
		default:
			if !hasRequestedChunking && requiresChunkingDependency(op) {
				appendOnce("static_analyzer")
				appendOnce("chunking")
			}
		}
		appendOnce(op)
	}
	return expanded
}

func requiresChunkingDependency(op string) bool {
	switch canonicalOperationName(op) {
	case "generate_summaries",
		"generate_topics",
		"generate_scene_blocks",
		"extract_entity",
		"extract_relation",
		"extract_entity_relation",
		"extract_inventory_items",
		"extract_metrics",
		"extract_provisions",
		"extract_semantic_projections",
		"extract_structured_knowledge":
		return true
	default:
		return false
	}
}
