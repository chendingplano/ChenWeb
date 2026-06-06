package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// GenerateTopicsProcessor re-runs the semantic chunking service (which produces
// the generate_topics status entry) as a standalone pipeline step.
type GenerateTopicsProcessor struct {
	InputStore  DocMetadataStore
	Service     chunkingHandler
	Logger      ApiTypes.JimoLogger
	ProcLogger  DocProcLogger
	Now         func() time.Time
	ArtifactDir string
}

func NewGenerateTopicsProcessor(
	inputStore DocMetadataStore,
	service chunkingHandler,
	logger ApiTypes.JimoLogger,
) *GenerateTopicsProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26051302")
	}
	return &GenerateTopicsProcessor{
		InputStore:  inputStore,
		Service:     service,
		Logger:      logger,
		ProcLogger:  DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:         time.Now,
		ArtifactDir: strings.TrimSpace(os.Getenv("ARTIFACT_DIR")),
	}
}

func (p *GenerateTopicsProcessor) Name() string { return "generate_topics" }

func (p *GenerateTopicsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	recordID := docProcLogRecordIDFromPayload(payload)
	var procErr error
	if svc, ok := p.Service.(topicGeneratingHandler); ok {
		procErr = runInputServiceEvent(
			ctx,
			payload,
			p.InputStore,
			p.Logger,
			svc.HandleGenerateTopicsInput,
			func(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error {
				if bh, ok := p.Service.(topicGeneratingBlockHandler); ok {
					return bh.HandleGenerateTopicsBlockInput(ctx, recordID, inputFilename, buf)
				}
				return errors.New("(MID_26052346) generate topics service does not support block input")
			},
		)
	} else {
		procErr = runChunkingServiceEvent(ctx, payload, p.InputStore, p.Service, p.Logger)
	}
	p.logSummary(ctx, recordID, start, p.Now(), procErr)
	return procErr
}

func (p *GenerateTopicsProcessor) logSummary(ctx context.Context, recordID *int64, start, end time.Time, procErr error) {
	if resolveDocProcLogDB(p.ProcLogger.DB) == nil {
		return
	}
	// Use a background context for DB writes: the pipeline context may already
	// be cancelled (e.g. due to a user stop), but the log must still be written.
	if ctx.Err() != nil {
		ctx = context.Background()
	}
	extraInfo, _ := json.Marshal(map[string]interface{}{})
	extraStr := docProcSummaryExtraInfoJSON(p.Service, extraInfo)
	msUsed := end.Sub(start).Milliseconds()
	finishExtraStr := generateTopicsFinishExtraInfoJSON(p.Service, msUsed)
	var errStr *string
	if procErr != nil {
		s := procErr.Error()
		errStr = &s
	}
	procProgress := docProcSummaryProgress(p.Service)
	activityName := "extract_topics"
	if err := p.ProcLogger.LogExtractTopicsFinish(ctx, DocProcLogRecord{
		CallReason:    "extract topics",
		DocProcName:   "generate_topics",
		RecordID:      recordID,
		ProcProgress:  nullableStringPtr(extractTopicsFinishProgress(procProgress)),
		ActivityName:  &activityName,
		ExtraInfoJSON: &finishExtraStr,
		Errors:        errStr,
		MSUsed:        &msUsed,
	}, "MID-26052856"); err != nil {
		p.Logger.Warn("failed to write extract_topics_finish log", "error", err)
	}
	if err := p.ProcLogger.LogSummary(ctx, "generate_topics", DocProcLogRecord{
		DocProcName:   p.Name(),
		ModelNames:    docProcSummaryModelNames(p.Service),
		PromptName:    strings.Join(docProcSummaryPromptNames(p.Service), ","),
		RecordID:      recordID,
		ExtraInfoJSON: &extraStr,
		Errors:        errStr,
		MSUsed:        &msUsed,
	}, "MID-26052857"); err != nil {
		p.Logger.Warn("failed to write doc_proc_summary log", "error", err)
	}
}

func generateTopicsFinishExtraInfoJSON(service any, totalTimeMs int64) string {
	extra := map[string]any{
		"total_time_ms": totalTimeMs,
	}
	if provider, ok := service.(docProcSummaryExtraInfoProvider); ok {
		if raw := provider.DocProcSummaryExtraInfo(); len(raw) > 0 {
			if value, ok := raw["total_topics"]; ok {
				extra["total_topics"] = value
			} else if value, ok := raw["topics_generated"]; ok {
				extra["total_topics"] = value
			}
			if value, ok := raw["total_chunks"]; ok {
				extra["total_chunks"] = value
			}
			if value, ok := raw["total_lines"]; ok {
				extra["total_lines"] = value
			}
		}
	}
	bs, err := json.Marshal(extra)
	if err != nil {
		return `{}`
	}
	return string(bs)
}

func extractTopicsFinishProgress(progress *string) string {
	if progress != nil {
		if value := strings.TrimSpace(*progress); value != "" {
			return value
		}
	}
	return "100%"
}
