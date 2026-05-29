package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type ChunkingProcessor struct {
	InputStore DocMetadataStore
	Service    interface {
		HandleInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error
	}
	Logger     ApiTypes.JimoLogger
	ProcLogger DocProcLogger
	Now        func() time.Time
}

func NewChunkingProcessor(
	inputStore DocMetadataStore,
	service interface {
		HandleInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error
	},
	logger ApiTypes.JimoLogger,
) *ChunkingProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26041902")
	}
	return &ChunkingProcessor{
		InputStore: inputStore,
		Service:    service,
		Logger:     logger,
		ProcLogger: DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:        time.Now,
	}
}

func (p *ChunkingProcessor) Name() string { return "chunking" }

func (p *ChunkingProcessor) LogName() string {
	if named, ok := p.Service.(interface{ LogName() string }); ok {
		if logName := strings.TrimSpace(named.LogName()); logName != "" {
			return logName
		}
	}
	return p.Name()
}

func (p *ChunkingProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	recordID := docProcLogRecordIDFromPayload(payload)
	procErr := runChunkingServiceEvent(ctx, payload, p.InputStore, p.Service, p.Logger)
	p.logSummary(ctx, recordID, start, p.Now(), procErr)
	return procErr
}

func (p *ChunkingProcessor) logSummary(ctx context.Context, recordID *int64, start, end time.Time, procErr error) {
	if p.ProcLogger.DB == nil {
		return
	}
	extraInfo, _ := json.Marshal(map[string]interface{}{})
	extraStr := docProcSummaryExtraInfoJSON(p.Service, extraInfo)
	var errStr *string
	if procErr != nil {
		s := procErr.Error()
		errStr = &s
	}
	if err := p.ProcLogger.LogSummary(ctx, DocProcLogRecord{
		DocProcName:   p.LogName(),
		ModelNames:    docProcSummaryModelNames(p.Service),
		PromptName:    strings.Join(docProcSummaryPromptNames(p.Service), ","),
		RecordID:      recordID,
		EntryType:     "doc_proc_summary",
		ExtraInfoJSON: &extraStr,
		Errors:        errStr,
		MSUsed:        int64Ptr(end.Sub(start).Milliseconds()),
	}, "MID-26052802"); err != nil {
		p.Logger.Warn("failed to write doc_proc_summary log", "error", err)
	}
}

func docProcLogRecordIDFromPayload(payload []byte) *int64 {
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil || evt.RecordID <= 0 {
		return nil
	}
	recordID := evt.RecordID
	return &recordID
}

func docProcSummaryModelNames(service any) []string {
	if provider, ok := service.(docProcModelNamesProvider); ok {
		return dedupeNonEmpty(provider.DocProcModelNames())
	}
	return nil
}

func docProcSummaryPromptNames(service any) []string {
	if provider, ok := service.(docProcPromptNamesProvider); ok {
		return dedupeNonEmpty(provider.DocProcPromptNames())
	}
	return nil
}

func docProcSummaryExtraInfoJSON(service any, fallback []byte) string {
	if provider, ok := service.(docProcSummaryExtraInfoProvider); ok {
		if extra := provider.DocProcSummaryExtraInfo(); len(extra) > 0 {
			if bs, err := json.Marshal(extra); err == nil {
				return string(bs)
			}
		}
	}
	return string(fallback)
}

func runChunkingServiceEvent(ctx context.Context, payload []byte, inputStore DocMetadataStore, service chunkingHandler, logger ApiTypes.JimoLogger) error {
	return runInputServiceEvent(
		ctx,
		payload,
		inputStore,
		logger,
		service.HandleInput,
		func(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error {
			if bh, ok := service.(chunkingBlockHandler); ok {
				return bh.HandleBlockInput(ctx, recordID, inputFilename, buf)
			}
			return fmt.Errorf("(MID_26052345) service does not support block input")
		},
	)
}

func runInputServiceEvent(
	ctx context.Context,
	payload []byte,
	inputStore DocMetadataStore,
	logger ApiTypes.JimoLogger,
	handleInput func(context.Context, int64, string, []byte) error,
	handleBlock func(context.Context, int64, string, *BlockBuffer) error,
) error {
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}

	rec, err := inputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if err == sql.ErrNoRows {
			logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		return fmt.Errorf("load kb.inputs record %d: %w", evt.RecordID, err)
	}

	inputFilename := strings.TrimSpace(evt.Filename)

	// Prefer the block buffer produced by the BlockingProcessor when available.
	if buf := BlockBufferFromContext(ctx); buf != nil {
		if inputFilename == "" {
			inputFilename = resolveChunkingInputFilename(rec, "")
		}
		if err := handleBlock(ctx, evt.RecordID, inputFilename, buf); err != nil {
			if !errors.Is(err, ErrPipelineStopped) {
				logger.Error("chunking processor failed (block input)", "record_id", rec.ID, "error", err)
			}
			return err
		}
		return nil
	}

	// Fall back to reading the input file directly.
	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		logger.Error("resolve chunking input path failed", "record_id", rec.ID, "error", err)
		return nil
	}
	fileBody, err := os.ReadFile(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Error("chunking input file not found", "record_id", rec.ID, "path", inputPath)
			return nil
		}
		return fmt.Errorf("read chunking input file %s: %w", inputPath, err)
	}

	if inputFilename == "" {
		inputFilename = resolveChunkingInputFilename(rec, inputPath)
	}
	if err := handleInput(ctx, evt.RecordID, inputFilename, fileBody); err != nil {
		if !errors.Is(err, ErrPipelineStopped) {
			logger.Error("chunking processor failed", "record_id", rec.ID, "error", err)
		}
		return err
	}
	return nil
}

func resolveChunkingInputFilename(rec DocMetadataInputRecord, inputPath string) string {
	candidates := []string{
		inputPath,
		rec.StagingFilename,
		rec.FileName,
		rec.ResultFilename,
	}
	for _, candidate := range candidates {
		name := filepath.Base(strings.TrimSpace(candidate))
		if name != "" && name != "." && name != string(filepath.Separator) {
			return name
		}
	}
	return ""
}
