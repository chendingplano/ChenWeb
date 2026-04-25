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
	evt, err := ParseLineFileGeneratedEvent(payload)
	if err != nil {
		return fmt.Errorf("parse event payload: %w", err)
	}
	if ShouldSkipLineFileGeneratedEvent(evt) {
		return nil
	}

	rec, err := p.InputStore.GetInputRecord(ctx, evt.RecordID)
	if err != nil {
		if err == sql.ErrNoRows {
			p.Logger.Error("kb.inputs record not found", "record_id", evt.RecordID)
			return nil
		}
		return fmt.Errorf("load kb.inputs record %d: %w", evt.RecordID, err)
	}

	inputPath, err := ResolveInputFilePath(evt, rec.ResultFilename, rec.ParserName, rec.StagingFilename)
	if err != nil {
		p.Logger.Error("resolve chunking input path failed", "record_id", rec.ID, "error", err)
		return nil
	}
	fileBody, err := os.ReadFile(inputPath)
	if err != nil {
		if os.IsNotExist(err) {
			p.Logger.Error("chunking input file not found", "record_id", rec.ID, "path", inputPath)
			return nil
		}
		return fmt.Errorf("read chunking input file %s: %w", inputPath, err)
	}

	inputFilename := strings.TrimSpace(evt.Filename)
	if inputFilename == "" {
		inputFilename = filepath.Base(inputPath)
	}
	if err := p.Service.HandleInput(ctx, evt.RecordID, inputFilename, fileBody); err != nil {
		p.Logger.Error("chunking processor failed", "record_id", rec.ID, "error", err)
		return err
	}
	return nil
}
