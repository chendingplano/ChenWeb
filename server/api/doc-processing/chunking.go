package docprocessing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	ChunkingMethodEnv = "CHUNKING_METHOD"
)

var allowedChunkingMethods = []string{
	ChunkingMethodFixed,
	ChunkingMethodTopic,
}

type chunkingHandler interface {
	HandleInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error
}

type chunkingBlockHandler interface {
	HandleBlockInput(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error
}

type topicGeneratingHandler interface {
	HandleGenerateTopicsInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error
}

type topicGeneratingBlockHandler interface {
	HandleGenerateTopicsBlockInput(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error
}

type summaryGeneratingHandler interface {
	HandleGenerateSummariesInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error
}

type summaryGeneratingBlockHandler interface {
	HandleGenerateSummariesBlockInput(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error
}

// topicsChunkBatcher allows per-chunk topic extraction by the batch coordinator.
// Implemented by FixedSizeChunkingService.
type topicsChunkBatcher interface {
	// ExtractTopicsForChunk extracts topics for one chunk. inputText must be
	// canonicalChunkInputText(chunk.Lines, docCtx) for cross-processor cache sharing.
	ExtractTopicsForChunk(ctx context.Context, recordID int64, chunk Chunk, totalChunks int, inputText string) ([]TopicItem, error)
	// FinalizeTopics dedupes, writes the artifact, indexes, embeds, and persists status.
	FinalizeTopics(ctx context.Context, recordID int64, inputFilename string, start time.Time, topics []TopicItem, chunks []Chunk) error
}

// summaryChunkBatcher allows per-chunk leaf-summary generation by the batch coordinator.
// Implemented by FixedSizeChunkingService.
type summaryChunkBatcher interface {
	// GenerateLeafSummaryForChunk generates one leaf-level summary. inputText must be
	// canonicalChunkInputText(chunk.Lines, docCtx) for cross-processor cache sharing.
	GenerateLeafSummaryForChunk(ctx context.Context, recordID int64, chunk Chunk, inputText string) (SummaryItem, error)
	// FinalizeSummaries builds the summary tree, writes artifacts, indexes, and persists status.
	FinalizeSummaries(ctx context.Context, recordID int64, inputFilename string, start time.Time, leafSummaries []SummaryItem, chunks []Chunk) error
}

type docProcModelNamesProvider interface {
	DocProcModelNames() []string
}

type docProcPromptNamesProvider interface {
	DocProcPromptNames() []string
}

type docProcSummaryExtraInfoProvider interface {
	DocProcSummaryExtraInfo() map[string]any
}

type ChunkingController struct {
	Method string
	Fixed  chunkingHandler
	Topic  chunkingHandler
}

func NewChunkingControllerFromEnv(fixed chunkingHandler, topic chunkingHandler) (*ChunkingController, error) {
	return NewChunkingController(strings.TrimSpace(os.Getenv(ChunkingMethodEnv)), fixed, topic)
}

func NewChunkingController(method string, fixed chunkingHandler, topic chunkingHandler) (*ChunkingController, error) {
	method = strings.ToLower(strings.TrimSpace(method))
	if method == "" {
		return nil, fmt.Errorf("(MID_26042112) missing %s", ChunkingMethodEnv)
	}

	switch method {
	case ChunkingMethodFixed:
		if fixed == nil {
			return nil, errors.New("(MID_26042113) fixed-size chunking service is nil")
		}
	case ChunkingMethodTopic:
		if topic == nil {
			return nil, errors.New("(MID_26042114) topic chunking service is nil")
		}
	default:
		return nil, fmt.Errorf(
			"(MID_26042115) unsupported %s: %s (allowed: %s)",
			ChunkingMethodEnv,
			method,
			strings.Join(allowedChunkingMethods, ", "),
		)
	}

	return &ChunkingController{
		Method: method,
		Fixed:  fixed,
		Topic:  topic,
	}, nil
}

func (c *ChunkingController) HandleInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error {
	switch strings.ToLower(strings.TrimSpace(c.Method)) {
	case ChunkingMethodFixed:
		return c.Fixed.HandleInput(ctx, recordID, inputFilename, inputFile)
	case ChunkingMethodTopic:
		return c.Topic.HandleInput(ctx, recordID, inputFilename, inputFile)
	default:
		return fmt.Errorf("(MID_26042116) unsupported chunking method: %s", c.Method)
	}
}

func (c *ChunkingController) HandleBlockInput(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error {
	switch strings.ToLower(strings.TrimSpace(c.Method)) {
	case ChunkingMethodFixed:
		if bh, ok := c.Fixed.(chunkingBlockHandler); ok {
			return bh.HandleBlockInput(ctx, recordID, inputFilename, buf)
		}
		return fmt.Errorf("(MID_26050630) fixed-size chunking service does not support block input")
	case ChunkingMethodTopic:
		if bh, ok := c.Topic.(chunkingBlockHandler); ok {
			return bh.HandleBlockInput(ctx, recordID, inputFilename, buf)
		}
		return fmt.Errorf("(MID_26050631) topic chunking service does not support block input")
	default:
		return fmt.Errorf("(MID_26050632) unsupported chunking method: %s", c.Method)
	}
}

func (c *ChunkingController) LogName() string {
	switch strings.ToLower(strings.TrimSpace(c.Method)) {
	case ChunkingMethodFixed:
		return "fix_size_chunking"
	case ChunkingMethodTopic:
		return "topic_chunking"
	default:
		return "chunking"
	}
}

func (c *ChunkingController) DocProcModelNames() []string {
	switch strings.ToLower(strings.TrimSpace(c.Method)) {
	case ChunkingMethodFixed:
		if provider, ok := c.Fixed.(docProcModelNamesProvider); ok {
			return provider.DocProcModelNames()
		}
	case ChunkingMethodTopic:
		if provider, ok := c.Topic.(docProcModelNamesProvider); ok {
			return provider.DocProcModelNames()
		}
	}
	return nil
}

func (c *ChunkingController) DocProcPromptNames() []string {
	switch strings.ToLower(strings.TrimSpace(c.Method)) {
	case ChunkingMethodFixed:
		if provider, ok := c.Fixed.(docProcPromptNamesProvider); ok {
			return provider.DocProcPromptNames()
		}
	case ChunkingMethodTopic:
		if provider, ok := c.Topic.(docProcPromptNamesProvider); ok {
			return provider.DocProcPromptNames()
		}
	}
	return nil
}

func (c *ChunkingController) DocProcSummaryExtraInfo() map[string]any {
	switch strings.ToLower(strings.TrimSpace(c.Method)) {
	case ChunkingMethodFixed:
		if provider, ok := c.Fixed.(docProcSummaryExtraInfoProvider); ok {
			return provider.DocProcSummaryExtraInfo()
		}
	case ChunkingMethodTopic:
		if provider, ok := c.Topic.(docProcSummaryExtraInfoProvider); ok {
			return provider.DocProcSummaryExtraInfo()
		}
	}
	return nil
}

func (c *ChunkingController) HandleGenerateTopicsInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error {
	switch strings.ToLower(strings.TrimSpace(c.Method)) {
	case ChunkingMethodFixed:
		if h, ok := c.Fixed.(topicGeneratingHandler); ok {
			return h.HandleGenerateTopicsInput(ctx, recordID, inputFilename, inputFile)
		}
		return c.Fixed.HandleInput(ctx, recordID, inputFilename, inputFile)
	case ChunkingMethodTopic:
		if h, ok := c.Topic.(topicGeneratingHandler); ok {
			return h.HandleGenerateTopicsInput(ctx, recordID, inputFilename, inputFile)
		}
		return c.Topic.HandleInput(ctx, recordID, inputFilename, inputFile)
	default:
		return fmt.Errorf("(MID_26052337) unsupported chunking method: %s", c.Method)
	}
}

func (c *ChunkingController) HandleGenerateTopicsBlockInput(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error {
	switch strings.ToLower(strings.TrimSpace(c.Method)) {
	case ChunkingMethodFixed:
		if h, ok := c.Fixed.(topicGeneratingBlockHandler); ok {
			return h.HandleGenerateTopicsBlockInput(ctx, recordID, inputFilename, buf)
		}
		if bh, ok := c.Fixed.(chunkingBlockHandler); ok {
			return bh.HandleBlockInput(ctx, recordID, inputFilename, buf)
		}
		return fmt.Errorf("(MID_26052338) fixed-size topic generation service does not support block input")
	case ChunkingMethodTopic:
		if h, ok := c.Topic.(topicGeneratingBlockHandler); ok {
			return h.HandleGenerateTopicsBlockInput(ctx, recordID, inputFilename, buf)
		}
		if bh, ok := c.Topic.(chunkingBlockHandler); ok {
			return bh.HandleBlockInput(ctx, recordID, inputFilename, buf)
		}
		return fmt.Errorf("(MID_26052339) topic generation service does not support block input")
	default:
		return fmt.Errorf("(MID_26052340) unsupported chunking method: %s", c.Method)
	}
}

func (c *ChunkingController) HandleGenerateSummariesInput(ctx context.Context, recordID int64, inputFilename string, inputFile []byte) error {
	switch strings.ToLower(strings.TrimSpace(c.Method)) {
	case ChunkingMethodFixed:
		if h, ok := c.Fixed.(summaryGeneratingHandler); ok {
			return h.HandleGenerateSummariesInput(ctx, recordID, inputFilename, inputFile)
		}
		return c.Fixed.HandleInput(ctx, recordID, inputFilename, inputFile)
	case ChunkingMethodTopic:
		if h, ok := c.Topic.(summaryGeneratingHandler); ok {
			return h.HandleGenerateSummariesInput(ctx, recordID, inputFilename, inputFile)
		}
		return c.Topic.HandleInput(ctx, recordID, inputFilename, inputFile)
	default:
		return fmt.Errorf("(MID_26052341) unsupported chunking method: %s", c.Method)
	}
}

func (c *ChunkingController) HandleGenerateSummariesBlockInput(ctx context.Context, recordID int64, inputFilename string, buf *BlockBuffer) error {
	switch strings.ToLower(strings.TrimSpace(c.Method)) {
	case ChunkingMethodFixed:
		if h, ok := c.Fixed.(summaryGeneratingBlockHandler); ok {
			return h.HandleGenerateSummariesBlockInput(ctx, recordID, inputFilename, buf)
		}
		if bh, ok := c.Fixed.(chunkingBlockHandler); ok {
			return bh.HandleBlockInput(ctx, recordID, inputFilename, buf)
		}
		return fmt.Errorf("(MID_26052342) fixed-size summary generation service does not support block input")
	case ChunkingMethodTopic:
		if h, ok := c.Topic.(summaryGeneratingBlockHandler); ok {
			return h.HandleGenerateSummariesBlockInput(ctx, recordID, inputFilename, buf)
		}
		if bh, ok := c.Topic.(chunkingBlockHandler); ok {
			return bh.HandleBlockInput(ctx, recordID, inputFilename, buf)
		}
		return fmt.Errorf("(MID_26052343) summary generation service does not support block input")
	default:
		return fmt.Errorf("(MID_26052344) unsupported chunking method: %s", c.Method)
	}
}
