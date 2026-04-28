package docprocessing

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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
