package docprocessing

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

type ChunkingProcessor struct {
	InputStore DocMetadataStore
	Service    interface {
		HandleInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error
	}
	Logger ApiTypes.JimoLogger
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
	return runChunkingServiceEvent(ctx, payload, p.InputStore, p.Service, p.Logger)
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
			logger.Error("chunking processor failed (block input)", "record_id", rec.ID, "error", err)
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
		logger.Error("chunking processor failed", "record_id", rec.ID, "error", err)
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
