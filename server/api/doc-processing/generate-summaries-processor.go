package docprocessing

import (
	"context"
	"errors"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// GenerateSummariesProcessor re-runs the fixed-size chunking service (which
// produces the generate_summaries status entry) as a standalone pipeline step.
type GenerateSummariesProcessor struct {
	InputStore DocMetadataStore
	Service    chunkingHandler
	Logger     ApiTypes.JimoLogger
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
	}
}

func (p *GenerateSummariesProcessor) Name() string { return "generate_summaries" }

func (p *GenerateSummariesProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	if svc, ok := p.Service.(summaryGeneratingHandler); ok {
		return runInputServiceEvent(
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
	}
	return runChunkingServiceEvent(ctx, payload, p.InputStore, p.Service, p.Logger)
}
