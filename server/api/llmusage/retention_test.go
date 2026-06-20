package llmusage

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRetentionRunDeletesOldUsageEventsAndArchives(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 6, 20, 10, 0, 0, 0, time.UTC)
	loc := time.UTC
	tmpDir := t.TempDir()

	oldDayPath := filepath.Join(tmpDir, "2026", "2026-05", "2026-05-20", "account-acct_1", "bodies")
	newDayPath := filepath.Join(tmpDir, "2026", "2026-06", "2026-06-19", "account-acct_1", "bodies")
	if err := os.MkdirAll(oldDayPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(oldDayPath) error = %v", err)
	}
	if err := os.MkdirAll(newDayPath, 0o755); err != nil {
		t.Fatalf("MkdirAll(newDayPath) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDayPath, "old.json.gz"), []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(old) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(newDayPath, "new.json.gz"), []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile(new) error = %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM llm_usage_event
WHERE workspace_day < $1`)).
		WithArgs(time.Date(2026, 5, 22, 0, 0, 0, 0, loc)).
		WillReturnResult(sqlmock.NewResult(0, 7))

	runner := &RetentionRunner{
		DB:            db,
		ArchiveRoot:   tmpDir,
		WorkspaceTZ:   loc,
		RetentionDays: 30,
		Now:           func() time.Time { return now },
	}

	result, err := runner.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.DeletedUsageEvents != 7 {
		t.Fatalf("DeletedUsageEvents = %d, want 7", result.DeletedUsageEvents)
	}
	if result.DeletedArchiveDays != 1 {
		t.Fatalf("DeletedArchiveDays = %d, want 1", result.DeletedArchiveDays)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "2026", "2026-05", "2026-05-20")); !os.IsNotExist(err) {
		t.Fatalf("expected old archive day removed, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "2026", "2026-06", "2026-06-19")); err != nil {
		t.Fatalf("expected recent archive day preserved, stat err = %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestRetentionCutoffDayUsesWorkspaceTimezone(t *testing.T) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}
	runner := &RetentionRunner{
		WorkspaceTZ:   loc,
		RetentionDays: 30,
		Now: func() time.Time {
			return time.Date(2026, 6, 20, 1, 30, 0, 0, time.UTC)
		},
	}

	got := runner.cutoffDay()
	want := time.Date(2026, 5, 21, 0, 0, 0, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("cutoffDay() = %v, want %v", got, want)
	}
}

func TestNextScheduledRunAtUsesWorkspaceTimezoneAndRunHour(t *testing.T) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("LoadLocation() error = %v", err)
	}
	now := time.Date(2026, 6, 20, 8, 15, 0, 0, time.UTC)

	got := nextScheduledRunAt(now, loc, 2)
	want := time.Date(2026, 6, 20, 2, 0, 0, 0, loc).Add(24 * time.Hour)
	if !got.Equal(want) {
		t.Fatalf("nextScheduledRunAt() = %v, want %v", got, want)
	}
}
