package docprocessing

import (
	"context"
	"strings"
	"testing"
)

type fakeChunkingHandler struct {
	calls int
}

func (f *fakeChunkingHandler) HandleInput(_ context.Context, _ int64, _ string, _ []byte) error {
	f.calls++
	return nil
}

func TestNewChunkingControllerFromEnv_RequiresMethod(t *testing.T) {
	t.Setenv(ChunkingMethodEnv, "")
	_, err := NewChunkingControllerFromEnv(&fakeChunkingHandler{}, &fakeChunkingHandler{})
	if err == nil {
		t.Fatalf("expected missing method error")
	}
	if !strings.Contains(err.Error(), "missing CHUNKING_METHOD") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChunkingController_DispatchesFixedMethod(t *testing.T) {
	fixed := &fakeChunkingHandler{}
	topic := &fakeChunkingHandler{}
	c, err := NewChunkingController(ChunkingMethodFixed, fixed, topic)
	if err != nil {
		t.Fatalf("NewChunkingController: %v", err)
	}
	if err := c.HandleInput(context.Background(), 1, "a.txt", []byte("x")); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	if fixed.calls != 1 {
		t.Fatalf("fixed calls=%d, want 1", fixed.calls)
	}
	if topic.calls != 0 {
		t.Fatalf("topic calls=%d, want 0", topic.calls)
	}
}

func TestChunkingController_DispatchesTopicMethod(t *testing.T) {
	fixed := &fakeChunkingHandler{}
	topic := &fakeChunkingHandler{}
	c, err := NewChunkingController(ChunkingMethodTopic, fixed, topic)
	if err != nil {
		t.Fatalf("NewChunkingController: %v", err)
	}
	if err := c.HandleInput(context.Background(), 1, "a.txt", []byte("x")); err != nil {
		t.Fatalf("HandleInput: %v", err)
	}
	if fixed.calls != 0 {
		t.Fatalf("fixed calls=%d, want 0", fixed.calls)
	}
	if topic.calls != 1 {
		t.Fatalf("topic calls=%d, want 1", topic.calls)
	}
}
