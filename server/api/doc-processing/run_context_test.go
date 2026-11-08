package docprocessing

import (
	"context"
	"testing"
)

func TestWithRunID_RoundTrip(t *testing.T) {
	ctx := withRunID(context.Background(), 42)
	got, ok := runIDFromContext(ctx)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != 42 {
		t.Fatalf("got=%d, want 42", got)
	}
}

func TestRunIDFromContext_NotSet(t *testing.T) {
	_, ok := runIDFromContext(context.Background())
	if ok {
		t.Fatal("expected ok=false for a context with no run id set")
	}
}

func TestTableRunIDDoesNotReplaceLLMExecution(t *testing.T) {
	ctx := withRunID(withLLMRunID(context.Background(), 1099), 87)
	if got := llmRunIDFromContext(ctx); got != 1099 {
		t.Fatalf("LLM run = %d, want 1099", got)
	}
	ctx = withLLMRunID(ctx, 0)
	if got := llmRunIDFromContext(ctx); got != 0 {
		t.Fatalf("failed execution retained old ID %d", got)
	}
}
