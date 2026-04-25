package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/fsnotify/fsnotify"
)

type testLogger struct{}

func (testLogger) Debug(string, ...any) {}
func (testLogger) Line(string, ...any)  {}
func (testLogger) Info(string, ...any)  {}
func (testLogger) Warn(string, ...any)  {}
func (testLogger) Error(string, ...any) {}
func (testLogger) Trace(string)         {}
func (testLogger) Close()               {}

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestRepoDirForRecordUsesThousandRecordShards(t *testing.T) {
	got, err := repoDirForRecord("/Users/cding/Apps/SemOS", 53)
	if err != nil {
		t.Fatalf("repoDirForRecord: %v", err)
	}
	want := filepath.Join("/Users/cding/Apps/SemOS", "Artifacts", "0", "53")
	if got != want {
		t.Fatalf("repoDirForRecord = %q, want %q", got, want)
	}
}

func TestStagingEventLoopProcessesCreateEventImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan fsnotify.Event, 1)
	errs := make(chan error, 1)
	fallback := make(chan time.Time)
	processed := make(chan struct{}, 1)

	go runStagingEventLoop(ctx, testLogger{}, events, errs, fallback, 10*time.Millisecond, func() error {
		processed <- struct{}{}
		return nil
	})

	events <- fsnotify.Event{Name: "doc.pdf", Op: fsnotify.Create}

	select {
	case <-processed:
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("expected create event to trigger staging processing immediately")
	}
}

func TestStagingEventLoopIgnoresChmodOnlyEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events := make(chan fsnotify.Event, 1)
	errs := make(chan error, 1)
	fallback := make(chan time.Time)
	processed := make(chan struct{}, 1)

	go runStagingEventLoop(ctx, testLogger{}, events, errs, fallback, 10*time.Millisecond, func() error {
		processed <- struct{}{}
		return nil
	})

	events <- fsnotify.Event{Name: "doc.pdf", Op: fsnotify.Chmod}

	select {
	case <-processed:
		t.Fatalf("expected chmod-only event to be ignored")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestUpsertStagedInputRecord_UpdatesExistingStagedRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	tmpDir := t.TempDir()
	srcPath := writeTempFile(t, tmpDir, "doc.pdf", "hello")

	updateSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET staging_filename = $1,
    md5 = $2,
    modify_time = NOW()
WHERE type = 'pdf'
  AND file_name = $3
  AND COALESCE(backup_filename, '') = ''
RETURNING id`)

	mock.ExpectQuery(updateSQL).
		WithArgs("doc.pdf", sqlmock.AnyArg(), srcPath).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(22)))

	recordID, updated, err := upsertStagedInputRecord(context.Background(), db, "doc.pdf", srcPath)
	if err != nil {
		t.Fatalf("upsertStagedInputRecord: %v", err)
	}
	if recordID != 22 {
		t.Fatalf("expected recordID=22, got %d", recordID)
	}
	if !updated {
		t.Fatalf("expected updated=true")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestProcessStagingOnceStoresFileUnderRecordShardedHomeDir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	tmpDir := t.TempDir()
	stagingDir := filepath.Join(tmpDir, "staging")
	backupDir := filepath.Join(tmpDir, "backup")
	homeDir := filepath.Join(tmpDir, "SemOS")
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("mkdir staging: %v", err)
	}
	srcPath := writeTempFile(t, stagingDir, "doc.pdf", "hello")
	backupPath := filepath.Join(backupDir, "doc.pdf")
	homePath := filepath.Join(homeDir, "Artifacts", "0", "53", "doc.pdf")

	updateSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET staging_filename = $1,
    md5 = $2,
    modify_time = NOW()
WHERE type = 'pdf'
  AND file_name = $3
  AND COALESCE(backup_filename, '') = ''
RETURNING id`)
	mock.ExpectQuery(updateSQL).
		WithArgs("doc.pdf", sqlmock.AnyArg(), srcPath).
		WillReturnError(sql.ErrNoRows)

	insertSQL := regexp.QuoteMeta(`
INSERT INTO kb.inputs (
    staging_filename,
    type,
    file_name,
    backup_filename,
    status,
    md5
) VALUES (
    $1,
    'pdf',
    $2,
    '',
    $3::jsonb,
    $4
)
RETURNING id`)
	mock.ExpectQuery(insertSQL).
		WithArgs("doc.pdf", srcPath, "[]", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(53)))

	finalizeSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET file_name = $1,
    backup_filename = $2,
    modify_time = NOW()
WHERE id = $3`)
	mock.ExpectExec(finalizeSQL).
		WithArgs(homePath, backupPath, int64(53)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := processStagingOnce(context.Background(), testLogger{}, db, stagingDir, backupDir, homeDir, nil); err != nil {
		t.Fatalf("processStagingOnce: %v", err)
	}

	if _, err := os.Stat(homePath); err != nil {
		t.Fatalf("expected staged file at sharded home path %q: %v", homePath, err)
	}
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Fatalf("expected staging source to be removed, stat err=%v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpsertStagedInputRecord_InsertsWhenNoExistingStagedRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	tmpDir := t.TempDir()
	srcPath := writeTempFile(t, tmpDir, "doc.pdf", "hello")

	updateSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET staging_filename = $1,
    md5 = $2,
    modify_time = NOW()
WHERE type = 'pdf'
  AND file_name = $3
  AND COALESCE(backup_filename, '') = ''
RETURNING id`)
	mock.ExpectQuery(updateSQL).
		WithArgs("doc.pdf", sqlmock.AnyArg(), srcPath).
		WillReturnError(sql.ErrNoRows)

	insertSQL := regexp.QuoteMeta(`
INSERT INTO kb.inputs (
    staging_filename,
    type,
    file_name,
    backup_filename,
    status,
    md5
) VALUES (
    $1,
    'pdf',
    $2,
    '',
    $3::jsonb,
    $4
)
RETURNING id`)
	mock.ExpectQuery(insertSQL).
		WithArgs("doc.pdf", srcPath, "[]", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(33)))

	recordID, updated, err := upsertStagedInputRecord(context.Background(), db, "doc.pdf", srcPath)
	if err != nil {
		t.Fatalf("upsertStagedInputRecord: %v", err)
	}
	if recordID != 33 {
		t.Fatalf("expected recordID=33, got %d", recordID)
	}
	if updated {
		t.Fatalf("expected updated=false")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpsertStagedInputRecord_UpdateErrorReturned(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	tmpDir := t.TempDir()
	srcPath := writeTempFile(t, tmpDir, "doc.pdf", "hello")

	updateSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET staging_filename = $1,
    md5 = $2,
    modify_time = NOW()
WHERE type = 'pdf'
  AND file_name = $3
  AND COALESCE(backup_filename, '') = ''
RETURNING id`)
	mock.ExpectQuery(updateSQL).
		WithArgs("doc.pdf", sqlmock.AnyArg(), srcPath).
		WillReturnError(fmt.Errorf("db down"))

	_, _, err = upsertStagedInputRecord(context.Background(), db, "doc.pdf", srcPath)
	if err == nil {
		t.Fatalf("expected error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
