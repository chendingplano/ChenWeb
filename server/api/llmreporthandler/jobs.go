package llmreporthandler

import (
	"context"
	"database/sql"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type DailyUsageReportRunner struct {
	DB           *sql.DB
	WorkspaceTZ  *time.Location
	TimezoneName string
	RunHour      int
	Now          func() time.Time
}

type DailyUsageRunResult struct {
	RowsAffected  int64
	DaysProcessed int
}

func (r *DailyUsageReportRunner) Run(ctx context.Context) error {
	_, err := r.RunWithResult(ctx)
	return err
}

func (r *DailyUsageReportRunner) RunWithResult(ctx context.Context) (DailyUsageRunResult, error) {
	store := NewStore(r.DB)
	var result DailyUsageRunResult
	for _, day := range r.targetDays() {
		rowsAffected, err := store.GenerateDailyUsageReport(ctx, day, r.timezoneName())
		if err != nil {
			return DailyUsageRunResult{}, err
		}
		result.RowsAffected += rowsAffected
		result.DaysProcessed++
	}
	return result, nil
}

func (r *DailyUsageReportRunner) targetDays() []time.Time {
	loc := r.location()
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	localNow := now().In(loc)
	today := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	yesterday := today.AddDate(0, 0, -1)
	return []time.Time{yesterday, today}
}

func (r *DailyUsageReportRunner) location() *time.Location {
	if r.WorkspaceTZ != nil {
		return r.WorkspaceTZ
	}
	return time.UTC
}

func (r *DailyUsageReportRunner) timezoneName() string {
	if tz := r.TimezoneName; tz != "" {
		return tz
	}
	return r.location().String()
}

func StartBackgroundDailyUsageReportGeneration(ctx context.Context, runner *DailyUsageReportRunner, logger ApiTypes.JimoLogger) {
	if runner == nil {
		return
	}
	go func() {
		runDailyUsageReportOnce(ctx, runner, logger)
		for {
			now := time.Now()
			nextRun := nextDailyReportRunAt(now, runner.location(), runner.RunHour)
			timer := time.NewTimer(time.Until(nextRun))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				runDailyUsageReportOnce(ctx, runner, logger)
			}
		}
	}()
}

func nextDailyReportRunAt(now time.Time, loc *time.Location, runHour int) time.Time {
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

func runDailyUsageReportOnce(ctx context.Context, runner *DailyUsageReportRunner, logger ApiTypes.JimoLogger) {
	if err := runner.Run(ctx); err != nil {
		if logger != nil {
			logger.Warn("llm daily usage report generation failed", "error", err)
		}
		return
	}
	if logger != nil {
		logger.Info("llm daily usage report generation completed",
			"timezone", runner.timezoneName())
	}
}
