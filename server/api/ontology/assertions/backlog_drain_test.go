package assertions

import (
	"context"
	"testing"
)

func TestDrainDeferredCandidatesRequiresDB(t *testing.T) {
	_, err := DrainDeferredCandidates(context.Background(), nil, 50)
	if err == nil {
		t.Fatal("expected error when db is nil")
	}
}
