package llmreconcile

import (
	"context"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func StartBackgroundReconciliation(ctx context.Context, runner *Runner, logger ApiTypes.JimoLogger, runHour int) {
	if runner == nil {
		return
	}
	go func() {
		runReconciliationOnce(ctx, runner, logger)
		for {
			nextRun := nextReconciliationRunAt(time.Now(), runner.location(), runHour)
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
	if err := runner.Run(ctx); err != nil {
		if logger != nil {
			logger.Warn("llm provider reconciliation failed", "error", err)
		}
		return
	}
	if logger != nil {
		logger.Info("llm provider reconciliation completed", "timezone", runner.timezoneName())
	}
}
