package docprocessing

import (
	"context"
	"database/sql"
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

type ControlService struct {
	Processors []Processor
	Logger     ApiTypes.JimoLogger
	InputStore DocMetadataStore
	Now        func() time.Time
}

func (s *ControlService) HandleEvent(ctx context.Context, payload []byte) {
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
		return
	}
	if s.Logger != nil {
		s.Logger.Info("start processing request",
			"record_id", evt.RecordID,
			"operations", evt.Operations,
		)
	}

	if ShouldSkipLineFileGeneratedEvent(evt) {
		if s.Logger != nil {
			s.Logger.Info("finish processing request",
				"record_id", evt.RecordID,
				"proc_status", "skipped",
				"ms_used", time.Since(requestStart).Milliseconds(),
			)
		}
		return
	}
	if !s.preflightInput(ctx, evt) {
		if s.Logger != nil {
			s.Logger.Info("finish processing request",
				"record_id", evt.RecordID,
				"proc_status", "failed",
				"ms_used", time.Since(requestStart).Milliseconds(),
			)
		}
		return
	}
	if len(evt.Operations) > 0 {
		processors = s.selectProcessors(evt.Operations)
		if len(processors) == 0 {
			if s.Logger != nil {
				s.Logger.Info("doc processor skipped, no matching operation", "operations", evt.Operations)
				s.Logger.Info("finish processing request",
					"record_id", evt.RecordID,
					"proc_status", "skipped",
					"ms_used", time.Since(requestStart).Milliseconds(),
				)
			}
			return
		}
	}

	requestFailed := false
	for _, p := range processors {
		if p == nil {
			continue
		}
		procStart := s.now()
		if s.Logger != nil {
			s.Logger.Info("start running processor",
				"record_id", evt.RecordID,
				"processor", p.Name(),
			)
		}
		if err := p.HandleEvent(ctx, payload); err != nil {
			requestFailed = true
			if s.Logger != nil {
				s.Logger.Error("doc processor failed", "processor", p.Name(), "error", err)
				s.Logger.Info("finish running processor",
					"record_id", evt.RecordID,
					"processor", p.Name(),
					"proc_status", "failed",
					"ms_used", time.Since(procStart).Milliseconds(),
				)
			}
			continue
		}
		if s.Logger != nil {
			s.Logger.Info("finish running processor",
				"record_id", evt.RecordID,
				"processor", p.Name(),
				"proc_status", "success",
				"ms_used", time.Since(procStart).Milliseconds(),
			)
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
		s.persistControlFailure(ctx, rec, errors.New("missing parser name"))
		return false
	}
	if strings.TrimSpace(rec.ResultFilename) == "" {
		s.persistControlFailure(ctx, rec, errors.New("missing result filename"))
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
			s.persistControlFailure(ctx, rec, fmt.Errorf("input file not exist: %s", inputPath))
			return false
		}
		s.persistControlFailure(ctx, rec, fmt.Errorf("stat input file: %w", err))
		return false
	}
	if fi.Size() == 0 {
		s.persistControlFailure(ctx, rec, errors.New("input file empty"))
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
