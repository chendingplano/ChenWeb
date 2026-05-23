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
	"strings"
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
	Now               func() time.Time
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

	go func() {
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
	processors = collapseRedundantChunkingProcessors(processors)

	// Inject a block buffer holder into context so the blocking processor
	// can share its output with downstream processors.
	ctx, _ = withBlockBufferHolder(ctx)

	requestFailed := false
	var firstErr error

	// Always run the blocking processor first, regardless of requested operations.
	if s.BlockingProcessor != nil {
		s.runSingleProcessor(ctx, payload, s.BlockingProcessor, evt.RecordID, &requestFailed, &firstErr)
	}

	for _, p := range processors {
		if p == nil {
			continue
		}
		s.runSingleProcessor(ctx, payload, p, evt.RecordID, &requestFailed, &firstErr)
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
	if err := p.HandleEvent(ctx, payload); err != nil {
		*requestFailed = true
		if *firstErr == nil {
			*firstErr = err
		}
		//if s.Logger != nil {
			s.Logger.Error("doc processor failed", "processor", processorName, "error", err)
			s.Logger.Info("finish running processor",
				"record_id", recordID,
				"processor", processorName,
				"proc_status", "failed",
				"ms_used", time.Since(procStart).Milliseconds(),
			)
		//}
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
		return collapseRedundantChunkingProcessors(s.Processors)
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
	return collapseRedundantChunkingProcessors(selected)
}

func collapseRedundantChunkingProcessors(processors []Processor) []Processor {
	if len(processors) == 0 {
		return processors
	}

	hasChunking := false
	for _, p := range processors {
		if p == nil {
			continue
		}
		if canonicalOperationName(p.Name()) == "chunking" {
			hasChunking = true
			break
		}
	}
	if !hasChunking {
		return processors
	}

	out := make([]Processor, 0, len(processors))
	for _, p := range processors {
		if p == nil {
			continue
		}
		switch canonicalOperationName(p.Name()) {
		case "generate_topics", "generate_summaries":
			continue
		default:
			out = append(out, p)
		}
	}
	return out
}
