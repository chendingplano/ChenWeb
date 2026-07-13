package docbenchmark

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestClassifyAttemptError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if l, f := classifyAttemptError(ctx, context.Background(), errors.New("x")); l != "canceled" || f != "canceled" {
		t.Fatalf("canceled = %s/%s", l, f)
	}
	if l, f := classifyAttemptError(context.Background(), context.Background(), ErrInvalidOutput); l != "failed" || f != "invalid_output" {
		t.Fatalf("invalid output = %s/%s", l, f)
	}
	if l, f := classifyAttemptError(context.Background(), context.Background(), &ScorerError{Err: errors.New("bad score")}); l != "failed" || f != "scorer_failed" {
		t.Fatalf("scorer = %s/%s", l, f)
	}
	if l, f := classifyAttemptError(context.Background(), context.Background(), &ProcessorError{Err: errors.New("bad processor")}); l != "failed" || f != "processor_failed" {
		t.Fatalf("processor = %s/%s", l, f)
	}
}

func TestClassifyTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	<-ctx.Done()
	if l, f := classifyAttemptError(ctx, context.Background(), context.DeadlineExceeded); l != "failed" || f != "timed_out" {
		t.Fatalf("timeout = %s/%s", l, f)
	}
}
