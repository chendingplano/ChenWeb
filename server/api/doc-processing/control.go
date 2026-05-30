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
		procErr := s.handleEvent(withEventID(ctx, eventID), payload)
		if s.EventStore == nil || strings.TrimSpace(eventID) == "" {
			return
		}
		if err := s.EventStore.UpsertConsumedStatus(context.Background(), eventID, procStart, time.Since(procStart).Milliseconds(), procErr); err != nil && s.Logger != nil {
			s.Logger.Error("failed upserting kb.events consumed status", "event_id", eventID, "error", err)
		}
	}()
	return nil
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

	// Always run the blocking processor first, regardless of requested operations.
	if s.BlockingProcessor != nil {
		s.runSingleProcessor(ctx, payload, s.BlockingProcessor, evt.RecordID, &requestFailed, &firstErr)
	}

	for _, p := range processors {
		if p == nil {
			continue
		}
		// Fast stop check: if the context was already cancelled by pollForStop,
		// skip remaining processors without attempting any LLM work.
		if isCtxStopped(ctx) {
			requestStopped = true
			if s.Logger != nil {
				s.Logger.Info("doc processor stop requested, halting pipeline", "record_id", evt.RecordID)
			}
			return nil
		}
		s.runSingleProcessor(ctx, payload, p, evt.RecordID, &requestFailed, &firstErr)
		// If a processor failed due to a user stop, treat it as stopped, not failed.
		if requestFailed && isCtxStopped(ctx) {
			requestFailed = false
			firstErr = nil
			requestStopped = true
			if s.Logger != nil {
				s.Logger.Info("doc processor stop detected after processor", "record_id", evt.RecordID, "processor", p.Name())
			}
			return nil
		}
	}
	if s.Logger != nil {
		status := "success"
		if requestFailed {
			status = "failed"
		}
		s.Logger.Info("finish processing request",
			"record_id", evt.RecordID,
			"proc_status", status,
			"ms_used", time.Since(requestStart).Milliseconds(),
		)
	}
	return firstErr
}

func (s *ControlService) runSingleProcessor(ctx context.Context, payload []byte, p Processor, recordID int64, requestFailed *bool, firstErr *error) {
	procStart := s.now()
	processorName := processorLogName(p)
	if s.Logger != nil {
		s.Logger.Info("start running processor",
			"record_id", recordID,
			"processor", processorName,
		)
	}
	s.persistPipelineStatus(ctx, recordID, "running", processorName, nil)
	if err := p.HandleEvent(ctx, payload); err != nil {
		*requestFailed = true
		if *firstErr == nil {
			*firstErr = err
		}
		procStatus := "failed"
		if errors.Is(err, ErrPipelineStopped) || isCtxStopped(ctx) {
			procStatus = "stopped"
			s.Logger.Info("processor stopped by user request", "processor", processorName, "record_id", recordID)
		} else {
			s.Logger.Error("doc processor failed", "processor", processorName, "error", err)
		}
		s.Logger.Info("finish running processor",
			"record_id", recordID,
			"processor", processorName,
			"proc_status", procStatus,
			"ms_used", time.Since(procStart).Milliseconds(),
		)
		return
	}
	if s.Logger != nil {
		s.Logger.Info("finish running processor",
			"record_id", recordID,
			"processor", processorName,
			"proc_status", "success",
			"ms_used", time.Since(procStart).Milliseconds(),
		)
	}
}

func (s *ControlService) persistPipelineStatus(ctx context.Context, recordID int64, procStatus string, processorName string, procErr error) {
	if s.InputStore == nil || recordID <= 0 {
		return
	}
	rec, err := s.InputStore.GetInputRecord(ctx, recordID)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("failed loading input record for doc pipeline status", "record_id", recordID, "error", err)
		}
		return
	}
	statusRaw, err := appendPipelineStatus(rec.StatusRaw, s.now(), procStatus, processorName, procErr)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("failed building doc pipeline status", "record_id", recordID, "error", err)
		}
		return
	}
	var errMsg *string
	if procErr != nil {
		msg := strings.TrimSpace(procErr.Error())
		errMsg = &msg
	}
	if err := s.InputStore.UpdateInputMetadata(ctx, recordID, DocMetadataUpdate{
		StatusRaw: statusRaw,
		ErrorMsg:  errMsg,
	}); err != nil && s.Logger != nil {
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

func (s *ControlService) preflightInput(ctx context.Context, evt LineFileGeneratedEvent) bool {
	if s.InputStore == nil {
		return true
	}
	rec, err := s.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
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

	if strings.TrimSpace(rec.ParserName) == "" {
		s.persistControlFailure(ctx, rec, errors.New("(MID_26042401) missing parser name"))
		return false
	}
	if strings.TrimSpace(rec.ResultFilename) == "" {
		s.persistControlFailure(ctx, rec, errors.New("(MID_26042402) missing result filename"))
		return false
	}

	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		s.persistControlFailure(ctx, rec, err)
		return false
	}
	fi, err := os.Stat(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.persistControlFailure(ctx, rec, fmt.Errorf("(MID_26042405) input file not exist: %s", inputPath))
			return false
		}
		s.persistControlFailure(ctx, rec, fmt.Errorf("(MID_26042406) stat input file: %w", err))
		return false
	}
	if fi.Size() == 0 {
		s.persistControlFailure(ctx, rec, errors.New("(MID_26042403) input file empty"))
		return false
	}
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

	selected := make([]Processor, 0, len(ops))
	for _, op := range ops {
		key := canonicalOperationName(op)
		if key == "" {
			continue
		}
		p, ok := available[key]
		if !ok {
			if s.Logger != nil {
				s.Logger.Info("doc processor operation ignored", "operation", op)
			}
			continue
		}
		selected = append(selected, p)
	}
	return selected
}
