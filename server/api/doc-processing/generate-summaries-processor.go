package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// GenerateSummariesProcessor re-runs the fixed-size chunking service (which
// produces the generate_summaries status entry) as a standalone pipeline step.
type GenerateSummariesProcessor struct {
	InputStore DocMetadataStore
	Service    chunkingHandler
	Logger     ApiTypes.JimoLogger
	ProcLogger DocProcLogger
	Now        func() time.Time
}

func NewGenerateSummariesProcessor(
	inputStore DocMetadataStore,
	service chunkingHandler,
	logger ApiTypes.JimoLogger,
) *GenerateSummariesProcessor {
	if logger == nil {
		logger = loggerutil.CreateDefaultLogger("MID_26051301")
	}
	return &GenerateSummariesProcessor{
		InputStore: inputStore,
		Service:    service,
		Logger:     logger,
		ProcLogger: DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:        time.Now,
	}
}

func (p *GenerateSummariesProcessor) Name() string { return "generate_summaries" }

func (p *GenerateSummariesProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
	recordID := docProcLogRecordIDFromPayload(payload)
	var procErr error
	if svc, ok := p.Service.(summaryGeneratingHandler); ok {
		procErr = runInputServiceEvent(
			ctx,
			payload,
			p.InputStore,
			p.Logger,
			svc.HandleGenerateSummariesInput,
			func(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error {
				if bh, ok := p.Service.(summaryGeneratingBlockHandler); ok {
					return bh.HandleGenerateSummariesBlockInput(ctx, recordID, inputFilename, buf)
				}
				return errors.New("(MID_26052347) generate summaries service does not support block input")
			},
		)
	} else {
		procErr = runChunkingServiceEvent(ctx, payload, p.InputStore, p.Service, p.Logger)
	}
	p.logSummary(ctx, recordID, start, p.Now(), procErr)
	return procErr
}

func (p *GenerateSummariesProcessor) logSummary(ctx context.Context, recordID *int64, start, end time.Time, procErr error) {
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
	finishExtraStr := generateSummaryFinishExtraInfoJSON(p.Service, msUsed)
	procProgress := docProcSummaryProgress(p.Service)
	var errStr *string
	if procErr != nil {
		s := procErr.Error()
		errStr = &s
	}
	activityName := "generate_summary"
	if err := p.ProcLogger.LogGenerateSummaryFinish(ctx, DocProcLogRecord{
		CallReason:    "generate summary",
		DocProcName:   "generate_summary",
		ModelNames:    docProcSummaryModelNames(p.Service),
		PromptName:    strings.Join(docProcSummaryPromptNames(p.Service), ","),
		RecordID:      recordID,
		ProcProgress:  procProgress,
		ActivityName:  &activityName,
		ExtraInfoJSON: &finishExtraStr,
		Errors:        errStr,
		MSUsed:        &msUsed,
	}, "MID-26052854"); err != nil {
		p.Logger.Warn("failed to write generate_summary_finish log", "error", err)
	}

	if err := p.ProcLogger.LogSummary(ctx, "generate_summary", DocProcLogRecord{
		DocProcName:   p.Name(),
		ModelNames:    docProcSummaryModelNames(p.Service),
		PromptName:    strings.Join(docProcSummaryPromptNames(p.Service), ","),
		RecordID:      recordID,
		ProcProgress:  procProgress,
		ExtraInfoJSON: &extraStr,
		Errors:        errStr,
		MSUsed:        &msUsed,
	}, "MID-26052855"); err != nil {
		p.Logger.Warn("failed to write doc_proc_summary log", "error", err)
	}
}

func generateSummaryFinishExtraInfoJSON(service any, totalTimeMs int64) string {
	extra := map[string]any{
		"total_time_ms": totalTimeMs,
	}
	if provider, ok := service.(docProcSummaryExtraInfoProvider); ok {
		if raw := provider.DocProcSummaryExtraInfo(); len(raw) > 0 {
			if value, ok := raw["num_summaries"]; ok {
				extra["num_summaries"] = value
			} else if value, ok := raw["summaries_generated"]; ok {
				extra["num_summaries"] = value
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

func docProcSummaryProgress(service any) *string {
	if provider, ok := service.(docProcSummaryExtraInfoProvider); ok {
		if raw := provider.DocProcSummaryExtraInfo(); len(raw) > 0 {
			if value := strings.TrimSpace(asString(raw["proc_progress"])); value != "" {
				return &value
			}
		}
	}
	return nil
}
