// Package scheduler runs periodic backlog-drain jobs (entity object
// resolution, ambiguous object resolution, search embedding backfill, ...)
// from user-defined schedules, closing the gap documented in ADR 2026070101:
// these jobs were previously only reachable as repeatable HTTP endpoints with
// nothing to actually call them on a schedule.
package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

// Schedule is one user-configured recurring job.
type Schedule struct {
	ID              int64
	Name            string
	JobType         string
	IntervalSeconds int
	Params          map[string]any
	Enabled         bool
	NextRunAt       time.Time
	LastRunAt       *time.Time
	LastRunStatus   string
}

// JobFunc executes one job type. It must be safe to call repeatedly and
// should itself be bounded (e.g. via a "limit" param) — the scheduler does
// not impose a timeout beyond the context it is given.
type JobFunc func(ctx context.Context, params map[string]any, logger ApiTypes.JimoLogger) (map[string]any, error)

// JobDescriptor names a job type for the "add schedule" UI and holds its
// implementation.
type JobDescriptor struct {
	Label string
	Run   JobFunc
}

// Registry maps job_type -> descriptor. Built once at startup by
// DefaultRegistry (registry.go) and passed to RunDueSchedules / StartScheduler.
type Registry map[string]JobDescriptor

// Store is the DB seam RunDueSchedules needs. SQLStore (scheduler_sql.go)
// implements it against kb.scheduled_jobs / kb.scheduled_job_runs.
type Store interface {
	DueSchedules(ctx context.Context, now time.Time, limit int) ([]Schedule, error)
	StartRun(ctx context.Context, scheduleID int64, jobType string) (runID int64, err error)
	FinishRun(ctx context.Context, runID int64, status string, result map[string]any, runErr error) error
	AdvanceSchedule(ctx context.Context, scheduleID int64, nextRunAt time.Time, status string) error
}

// RunSummary summarizes one RunDueSchedules pass.
type RunSummary struct {
	Scanned   int
	Succeeded int
	Failed    int
}

const (
	statusSuccess = "success"
	statusFailed  = "failed"
)

// RunDueSchedules loads every schedule due at or before now (enabled, with
// next_run_at <= now) and runs each one. A schedule with an unregistered
// job_type is recorded as a failed run — not silently skipped — so it is
// visible in run history, and is still rescheduled so it does not spin on
// every tick. Each schedule is processed independently: one failure does not
// stop the others.
func RunDueSchedules(ctx context.Context, store Store, registry Registry, now time.Time, logger ApiTypes.JimoLogger) (RunSummary, error) {
	var summary RunSummary
	schedules, err := store.DueSchedules(ctx, now, 50)
	if err != nil {
		return summary, err
	}
	summary.Scanned = len(schedules)

	for _, sched := range schedules {
		runID, err := store.StartRun(ctx, sched.ID, sched.JobType)
		if err != nil {
			summary.Failed++
			if logger != nil {
				logger.Warn("scheduler: start run failed", "schedule_id", sched.ID, "error", err.Error())
			}
			continue
		}

		var (
			result map[string]any
			runErr error
		)
		if descriptor, ok := registry[sched.JobType]; ok {
			result, runErr = descriptor.Run(ctx, sched.Params, logger)
		} else {
			runErr = fmt.Errorf("unknown job_type %q", sched.JobType)
		}

		status := statusSuccess
		if runErr != nil {
			status = statusFailed
			summary.Failed++
			if logger != nil {
				logger.Warn("scheduler: job failed", "schedule_id", sched.ID, "job_type", sched.JobType, "error", runErr.Error())
			}
		} else {
			summary.Succeeded++
		}

		if err := store.FinishRun(ctx, runID, status, result, runErr); err != nil && logger != nil {
			logger.Warn("scheduler: finish run failed", "run_id", runID, "error", err.Error())
		}
		nextRunAt := now.Add(time.Duration(sched.IntervalSeconds) * time.Second)
		if err := store.AdvanceSchedule(ctx, sched.ID, nextRunAt, status); err != nil && logger != nil {
			logger.Warn("scheduler: advance schedule failed", "schedule_id", sched.ID, "error", err.Error())
		}
	}
	return summary, nil
}

// StartScheduler runs RunDueSchedules on a fixed tick until ctx is done.
// Mirrors the agentplatformhandler.StartWorkers precedent: a background
// goroutine started from main.go, not a new kind of process.
func StartScheduler(ctx context.Context, store Store, registry Registry, tickInterval time.Duration, logger ApiTypes.JimoLogger) {
	go func() {
		ticker := time.NewTicker(tickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if _, err := RunDueSchedules(ctx, store, registry, now, logger); err != nil && logger != nil {
					logger.Warn("scheduler: run due schedules failed", "error", err.Error())
				}
			}
		}
	}()
}
