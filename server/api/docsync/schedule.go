package docsync

import (
	"context"
	"time"

	"github.com/chendingplano/deepdoc/server/processhealth"
)

// Schedule keeps each start on the configured local wall-clock time across DST.
// A pending transfer from an earlier run can resume when the service starts.
func (r *Runner) Schedule(ctx context.Context, health *processhealth.Reporter) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go r.refreshHealth(ctx, health)
	previousStart, _ := r.Window.bounds(time.Now())
	last := r.RunWithHealth(ctx, health)
	for ctx.Err() == nil {
		if last == nil {
			_ = health.Set(processhealth.Waiting, "outside_sync_window", "document sync", "Waiting for the next sync window", processhealth.None)
		}
		now := time.Now()
		start, end := r.Window.bounds(now)
		if now.Before(end) && start.After(previousStart) {
			previousStart = start
			last = r.RunWithHealth(ctx, health)
			continue
		}
		next := r.Window.Next(now)
		if err := wait(ctx, time.Until(next)); err != nil {
			break
		}
		previousStart = next
		last = r.RunWithHealth(ctx, health)
	}
	return ctx.Err()
}

func (r *Runner) refreshHealth(ctx context.Context, health *processhealth.Reporter) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := health.Refresh(); err != nil {
				r.Logger.Warn("refresh document sync health report", "error", err)
			}
		}
	}
}

func (r *Runner) RunWithHealth(ctx context.Context, health *processhealth.Reporter) error {
	_ = health.Set(processhealth.Healthy, "", "", "", processhealth.None)
	runner := *r
	original := runner.SSH.OnRetry
	var retries int
	runner.SSH.OnRetry = func(ctx context.Context, runID int64, segment int, offset int64, err error) {
		retries++
		if logErr := r.event(ctx, runID, "segment_retried", workItem{}, 0, map[string]any{
			"segment": segment, "offset": offset, "reason": err.Error(),
		}); logErr != nil {
			r.Logger.Error("record transfer retry", "error", logErr)
		}
		if retries >= 3 {
			_ = health.Set(processhealth.Degraded, "transfer_retrying", "document transfer", "Document sync is retrying the production connection", processhealth.None)
		}
		if original != nil {
			original(ctx, runID, segment, offset, err)
		}
	}
	err := runner.Run(ctx)
	if err != nil {
		_ = health.Set(processhealth.Degraded, "sync_failed", "document sync", "Document sync failed. See the sync activity log for details", processhealth.OperatorAction)
	} else {
		_ = health.Set(processhealth.Healthy, "", "", "", processhealth.None)
	}
	return err
}
