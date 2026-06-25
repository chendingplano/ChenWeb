package docprocessing

import (
	"strings"
	"testing"
)

// BuildReviewerToolClient must error cleanly (no panic, nil client) when the
// model ref cannot be resolved. The success path requires a real MODEL_DEF_FILE
// and is exercised by the doc-review smoke test, not here.
func TestBuildReviewerToolClientEmptyRefErrors(t *testing.T) {
	client, modelName, err := BuildReviewerToolClient("")
	if err == nil {
		t.Fatalf("expected error for empty model ref, got nil")
	}
	if client != nil {
		t.Fatalf("expected nil client on error, got %T", client)
	}
	if modelName != "" {
		t.Fatalf("expected empty model name on error, got %q", modelName)
	}
}

func TestBuildReviewerToolClientUnknownRefErrors(t *testing.T) {
	_, _, err := BuildReviewerToolClient("no-such-model-ref-xyz")
	if err == nil {
		t.Fatalf("expected error for unknown model ref, got nil")
	}
	// The error should reference the resolution failure (missing file or model),
	// not panic — exact text depends on whether MODEL_DEF_FILE is set.
	if strings.TrimSpace(err.Error()) == "" {
		t.Fatalf("expected a non-empty error message")
	}
}
