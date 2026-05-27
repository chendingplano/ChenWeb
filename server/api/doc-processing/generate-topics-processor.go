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

// GenerateTopicsProcessor re-runs the semantic chunking service (which produces
// the generate_topics status entry) as a standalone pipeline step.
type GenerateTopicsProcessor struct {
	InputStore DocMetadataStore
	Service    chunkingHandler
	Logger     ApiTypes.JimoLogger
	ProcLogger DocProcLogger
	Now        func() time.Time
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
		InputStore: inputStore,
		Service:    service,
		Logger:     logger,
		ProcLogger: DocProcLogger{DB: ApiTypes.ProjectDBHandle},
		Now:        time.Now,
	}
}

func (p *GenerateTopicsProcessor) Name() string { return "generate_topics" }

func (p *GenerateTopicsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	start := p.Now()
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
	p.logSummary(ctx, start, p.Now(), procErr)
	return procErr
}

func (p *GenerateTopicsProcessor) logSummary(ctx context.Context, start, end time.Time, procErr error) {
	if resolveDocProcLogDB(p.ProcLogger.DB) == nil {
		return
	}
	extraInfo, _ := json.Marshal(map[string]interface{}{})
	extraStr := docProcSummaryExtraInfoJSON(p.Service, extraInfo)
	msUsed := end.Sub(start).Milliseconds()
	var errStr *string
	if procErr != nil {
		s := procErr.Error()
		errStr = &s
	}
	if err := p.ProcLogger.LogSummary(ctx, DocProcLogRecord{
		DocProcName:   p.Name(),
		ModelNames:    docProcSummaryModelNames(p.Service),
		PromptName:    strings.Join(docProcSummaryPromptNames(p.Service), ","),
		EntryType:     "doc_proc_summary",
		ExtraInfoJSON: &extraStr,
		Errors:        errStr,
		MSUsed:        &msUsed,
	}); err != nil {
		p.Logger.Warn("failed to write doc_proc_summary log", "error", err)
	}
}
