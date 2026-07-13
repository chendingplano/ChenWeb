package docbenchmark

// runner.go contains the deliberately small, synchronous attempt state
// machine.  Process isolation and scheduling belong to orchestrator/worker;
// this type only owns one logical case and therefore is straightforward to
// exercise without starting a production processor.

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"
)

// ProcessorError and ScorerError preserve the failure taxonomy while allowing
// callers to wrap the underlying error.
type ProcessorError struct{ Err error }

func (e *ProcessorError) Error() string { return "processor: " + e.Err.Error() }
func (e *ProcessorError) Unwrap() error { return e.Err }

type ScorerError struct{ Err error }

func (e *ScorerError) Error() string { return "scorer: " + e.Err.Error() }
func (e *ScorerError) Unwrap() error { return e.Err }

type RunnerConfig struct {
	Owner                            string
	Timeout, Heartbeat, AttemptLease time.Duration
	MaxAttempts                      int
	RetainWorkspaces                 bool
	Now                              func() time.Time
}

// AttemptWork is supplied by a worker. Execute must invoke the production
// controller for execution attempts; it is never called for a rescore.
type AttemptWork struct {
	Execute   func(context.Context, AttemptRecord) error
	Capture   func(context.Context, AttemptRecord) (any, error)
	Reconcile func(any) (any, error)
	Score     func(context.Context, AttemptRecord, any) error
	Cleanup   func(context.Context, AttemptRecord) error
}

type Runner struct {
	Store  SQLStore
	Config RunnerConfig
	Work   AttemptWork
}

func (r Runner) now() time.Time {
	if r.Config.Now != nil {
		return r.Config.Now().UTC()
	}
	return time.Now().UTC()
}

// RunCase claims and processes attempts until one is selected or the attempt
// budget is exhausted. A false claim means another worker owns the case (or
// it is already terminal), and is intentionally not an error.
func (r Runner) RunCase(ctx context.Context, caseRunID string) error {
	if r.Store.DB == nil {
		return errors.New("runner: nil database")
	}
	if r.Config.Owner == "" {
		return errors.New("runner: empty owner")
	}
	if r.Config.Timeout <= 0 {
		return errors.New("runner: timeout must be positive")
	}
	if r.Config.Heartbeat <= 0 {
		r.Config.Heartbeat = r.Config.Timeout / 3
		if r.Config.Heartbeat <= 0 {
			r.Config.Heartbeat = time.Second
		}
	}
	if r.Config.AttemptLease <= r.Config.Timeout {
		return errors.New("runner: attempt lease must exceed timeout")
	}
	for {
		claim, err := r.Store.ClaimAttempt(ctx, caseRunID, r.Config.Owner, r.now(), r.Config.AttemptLease, r.Config.MaxAttempts)
		if err != nil {
			return err
		}
		if !claim.Claimed {
			return nil
		}
		if err = r.runAttempt(ctx, claim.Attempt); err != nil {
			return err
		}
		// runAttempt records the terminal state. SelectAttempt is deliberately
		// called here, after all evidence/score writes have completed.
		// Failed infrastructure/scorer attempts are retried immediately by the
		// next ClaimAttempt; processor-quality failures are selected instead.
		if r.retryableAttempt(claim.Attempt.ID) {
			continue
		}
		if err = r.selectIfTerminal(ctx, caseRunID, claim.Attempt.ID); err != nil {
			return err
		}
		return nil
	}
}

// retryableAttempt is conservative: the database remains authoritative for
// whether a retry can be claimed, while this method avoids spinning after
// quality/cancellation outcomes. A failed attempt's details are read only for
// the retry decision.
func (r Runner) retryableAttempt(id string) bool {
	var lifecycle, failure string
	if r.Store.DB.QueryRow(`SELECT lifecycle,COALESCE(failure_kind,'') FROM kb.benchmark_case_attempts WHERE id=$1`, id).Scan(&lifecycle, &failure) != nil {
		return false
	}
	return lifecycle == "failed" && (failure == "infrastructure_failed" || failure == "stale_lease" || failure == "scorer_failed" || failure == "timed_out")
}

func (r Runner) selectIfTerminal(ctx context.Context, caseRunID, id string) error {
	var lifecycle string
	if err := r.Store.DB.QueryRowContext(ctx, `SELECT lifecycle FROM kb.benchmark_case_attempts WHERE id=$1`, id).Scan(&lifecycle); err != nil {
		return err
	}
	if lifecycle == "succeeded" || lifecycle == "failed" || lifecycle == "canceled" {
		return r.Store.SelectAttempt(ctx, caseRunID, id)
	}
	return nil
}

func (r Runner) runAttempt(parent context.Context, attempt AttemptRecord) error {
	ctx, cancel := context.WithTimeout(parent, r.Config.Timeout)
	defer cancel()
	var stopped atomic.Bool
	done := make(chan struct{})
	go func() {
		t := time.NewTicker(r.Config.Heartbeat)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if stopped.Load() {
					return
				}
				_ = r.Store.HeartbeatAttempt(context.Background(), attempt.ID, r.Config.Owner, r.now().Add(r.Config.AttemptLease), map[string]any{"phase": "running"})
			case <-done:
				return
			}
		}
	}()
	defer func() { stopped.Store(true); close(done) }()

	var err error
	if attempt.Kind == "execution" {
		if r.Work.Execute == nil {
			err = errors.New("runner: missing executor")
		} else {
			err = r.Work.Execute(ctx, attempt)
		}
		if err == nil {
			if r.Work.Capture == nil {
				err = errors.New("runner: missing capture")
			}
		}
	} else if attempt.Kind != "rescore" {
		err = fmt.Errorf("runner: unknown attempt kind %q", attempt.Kind)
	}
	var captured any
	captureVerified := false
	if err == nil {
		// Capture returns only after the adapter has copied and hash-verified
		// canonical evidence.  Keep that fact even when reconciliation or the
		// scorer subsequently fails: those failures must rescore the evidence,
		// never invoke the production controller again.
		captured, err = r.Work.Capture(ctx, attempt)
		captureVerified = err == nil
	}
	if err == nil && r.Work.Reconcile != nil {
		captured, err = r.Work.Reconcile(captured)
	}
	if err == nil && r.Work.Score != nil {
		err = r.Work.Score(ctx, attempt, captured)
	}
	verified := captureVerified
	lifecycle, failure := classifyAttemptError(ctx, parent, err)
	if verified {
		lifecycle, failure = "succeeded", ""
	}
	if e := r.Store.FinishAttempt(context.Background(), attempt.ID, r.Config.Owner, lifecycle, failure, max(0, time.Since(attempt.StartedAt.Time).Milliseconds()), verified); e != nil {
		return e
	}
	if verified && !r.Config.RetainWorkspaces && r.Work.Cleanup != nil {
		if e := r.Work.Cleanup(context.Background(), attempt); e != nil {
			return e
		}
	}
	return nil
}

func classifyAttemptError(ctx, parent context.Context, err error) (string, string) {
	if err == nil {
		return "succeeded", ""
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "failed", "timed_out"
	}
	if errors.Is(parent.Err(), context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return "canceled", "canceled"
	}
	var pe *ProcessorError
	if errors.As(err, &pe) {
		return "failed", "processor_failed"
	}
	var se *ScorerError
	if errors.As(err, &se) {
		return "failed", "scorer_failed"
	}
	if errors.Is(err, ErrInvalidOutput) {
		return "failed", "invalid_output"
	}
	return "failed", "infrastructure_failed"
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
