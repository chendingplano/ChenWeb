package docprocessing

import (
	"context"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// GenerateTopicsProcessor re-runs the semantic chunking service (which produces
// the generate_topics status entry) as a standalone pipeline step.
type GenerateTopicsProcessor struct {
	InputStore DocMetadataStore
	Service    chunkingHandler
	Logger     ApiTypes.JimoLogger
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
	}
}

func (p *GenerateTopicsProcessor) Name() string { return "generate_topics" }

func (p *GenerateTopicsProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	return runChunkingServiceEvent(ctx, payload, p.InputStore, p.Service, p.Logger)
}
