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

// withRunID is the single choke point where handleEvent tags ctx with the
// kb.doc_process_runs.id for a pipeline invocation (control.go:862). Every
// processor's LLM calls read the run id back out via llmRunIDFromContext
// (llm_capture_input.go) to populate llm_usage_event.run_id, so withRunID
// must also satisfy that reader or every LLM call made during a plain
// doc-processing run (e.g. extract_metrics) silently gets run_id=NULL.
func TestWithRunID_AlsoTagsLLMRunID(t *testing.T) {
	ctx := withRunID(context.Background(), 87)
	if got := llmRunIDFromContext(ctx); got != 87 {
		t.Fatalf("llmRunIDFromContext() = %d, want 87", got)
	}
}
