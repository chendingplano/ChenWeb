package docprocessing

import (
	"context"

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
	return runChunkingServiceEvent(ctx, payload, p.InputStore, p.Service, p.Logger)
}
