package llmusage

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type RetentionRunner struct {
	DB            *sql.DB
	ArchiveRoot   string
	WorkspaceTZ   *time.Location
	RetentionDays int
	Now           func() time.Time
}

type RetentionResult struct {
	DeletedUsageEvents int64
	DeletedArchiveDays int
}

func (r *RetentionRunner) Run(ctx context.Context) (RetentionResult, error) {
	cutoffDay := r.cutoffDay()
	deletedRows, err := r.deleteOldUsageEvents(ctx, cutoffDay)
	if err != nil {
		return RetentionResult{}, err
	}
	deletedDirs, err := r.deleteOldArchiveDays(cutoffDay)
	if err != nil {
		return RetentionResult{}, err
	}
	return RetentionResult{
		DeletedUsageEvents: deletedRows,
		DeletedArchiveDays: deletedDirs,
	}, nil
}

func (r *RetentionRunner) cutoffDay() time.Time {
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	loc := time.UTC
	if r.WorkspaceTZ != nil {
		loc = r.WorkspaceTZ
	}
	retentionDays := r.RetentionDays
	if retentionDays <= 0 {
		retentionDays = 30
	}
	workspaceNow := now().In(loc)
	workspaceDay := time.Date(workspaceNow.Year(), workspaceNow.Month(), workspaceNow.Day(), 0, 0, 0, 0, loc)
	return workspaceDay.AddDate(0, 0, -(retentionDays - 1))
}

func (r *RetentionRunner) deleteOldUsageEvents(ctx context.Context, cutoffDay time.Time) (int64, error) {
	if r.DB == nil {
		return 0, nil
	}
	const stmt = `DELETE FROM llm_usage_event
WHERE workspace_day < $1`
	res, err := r.DB.ExecContext(ctx, stmt, cutoffDay)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *RetentionRunner) deleteOldArchiveDays(cutoffDay time.Time) (int, error) {
	root := strings.TrimSpace(r.ArchiveRoot)
	if root == "" {
		return 0, nil
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !info.IsDir() {
		return 0, nil
	}

	deleted := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return nil
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 3 {
			return nil
		}
		day, err := time.ParseInLocation("2006-01-02", parts[2], cutoffDay.Location())
		if err != nil {
			return nil
		}
		if day.Before(cutoffDay) {
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			deleted++
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func nextScheduledRunAt(now time.Time, loc *time.Location, runHour int) time.Time {
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

func StartBackgroundRetention(ctx context.Context, runner *RetentionRunner, logger ApiTypes.JimoLogger, loc *time.Location, runHour int) {
	if runner == nil {
		return
	}
	go func() {
		runRetentionOnce(ctx, runner, logger)
		for {
			now := time.Now()
			nextRun := nextScheduledRunAt(now, loc, runHour)
			timer := time.NewTimer(time.Until(nextRun))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				runRetentionOnce(ctx, runner, logger)
			}
		}
	}()
}

func runRetentionOnce(ctx context.Context, runner *RetentionRunner, logger ApiTypes.JimoLogger) {
	result, err := runner.Run(ctx)
	if err != nil {
		if logger != nil {
			logger.Warn("llm usage retention failed", "error", err)
		}
		return
	}
	if logger != nil {
		logger.Info("llm usage retention completed",
			"deleted_usage_events", result.DeletedUsageEvents,
			"deleted_archive_days", result.DeletedArchiveDays)
	}
}
