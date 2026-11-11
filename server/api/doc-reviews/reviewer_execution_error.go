package docreviews

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// reviewerExecutionError keeps a failure tied to its independent aspect.
type reviewerExecutionError struct {
	Aspect string
	Err    error
}

func (e *reviewerExecutionError) Error() string { return fmt.Sprintf("%s: %v", e.Aspect, e.Err) }
func (e *reviewerExecutionError) Unwrap() error { return e.Err }

// finishReviewAspects publishes terminal status only after results are durable.
func (p *ReviewProcessor) finishReviewAspects(ctx context.Context, runID int64, runners []reviewRunner, reviewErrors []error) []error {
	store, ok := p.StatusStore.(ReviewAspectResultStore)
	if !ok {
		return nil
	}
	statusCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	var statusErrors []error
	for _, runner := range runners {
		aspect := runner.reviewer.Name()
		var aspectErr error
		for _, err := range reviewErrors {
			var unitErr *reviewerExecutionError
			if !errors.As(err, &unitErr) || unitErr.Aspect == aspect {
				aspectErr = err
				break
			}
		}
		if err := store.FinishAspect(statusCtx, runID, aspect, aspectErr); err != nil {
			p.Logger.Error("reviewer terminal status failed", "run_id", runID, "aspect", aspect, "error", err)
			statusErrors = append(statusErrors, &reviewerExecutionError{Aspect: aspect, Err: err})
		}
	}
	return statusErrors
}
