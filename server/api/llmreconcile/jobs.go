package llmreconcile

import (
	"context"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// StartBackgroundReconciliation captures official provider balances once per
// hour. runHour remains in the signature for backwards-compatible callers;
// daily reporting still uses it, but balance history must be hourly.
func StartBackgroundReconciliation(ctx context.Context, runner *Runner, logger ApiTypes.JimoLogger, runHour int) {
	if runner == nil {
		return
	}
	go func() {
		runReconciliationOnce(ctx, runner, logger)
		for {
			nextRun := nextHourlyReconciliationRunAt(time.Now(), runner.location())
			timer := time.NewTimer(time.Until(nextRun))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				runReconciliationOnce(ctx, runner, logger)
			}
		}
	}()
}

func nextHourlyReconciliationRunAt(now time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	localNow := now.In(loc)
	return time.Date(localNow.Year(), localNow.Month(), localNow.Day(), localNow.Hour()+1, 0, 0, 0, loc)
}

func nextReconciliationRunAt(now time.Time, loc *time.Location, runHour int) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	if runHour < 0 || runHour > 23 {
		runHour = 2
	}
	localNow := now.In(loc)
	next := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), runHour, 0, 0, 0, loc)
	if !next.After(localNow) {
		next = next.Add(24 * time.Hour)
	}
	return next
}

func runReconciliationOnce(ctx context.Context, runner *Runner, logger ApiTypes.JimoLogger) {
	result, err := runner.RunWithResult(ctx)
	if err != nil {
		if logger != nil {
			logger.Warn("llm provider reconciliation run failed", "error", err)
		}
		return
	}
	for _, failure := range result.Failures {
		if logger != nil {
			logger.Warn("llm provider reconciliation failed for account",
				"account_id", failure.AccountID, "account_name", failure.AccountName, "error", failure.Err)
		}
	}
	if logger != nil {
		logger.Info("llm provider reconciliation completed",
			"timezone", runner.timezoneName(),
			"accounts_considered", result.AccountsConsidered,
			"snapshots_created", result.SnapshotsCreated,
			"failures", len(result.Failures))
	}
}
