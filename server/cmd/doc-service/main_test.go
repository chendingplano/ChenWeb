package main

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir temp file parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func writeTempZipFile(t *testing.T, dir, name string, files map[string]string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip file: %v", err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	for entryName, content := range files {
		w, err := zw.Create(entryName)
		if err != nil {
			t.Fatalf("create zip entry %q: %v", entryName, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %q: %v", entryName, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
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
WHERE file_name = $3
  AND COALESCE(backup_filename, '') = ''
RETURNING id`)

	mock.ExpectQuery(updateSQL).
		WithArgs("doc.pdf", sqlmock.AnyArg(), srcPath).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(22)))

	recordID, updated, err := upsertStagedInputRecord(context.Background(), db, "doc.pdf", srcPath, "pdf")
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
	homePath := filepath.Join(homeDir, "Artifacts", "0", "53", "doc.pdf")
	relativeHomePath := filepath.Join("Artifacts", "0", "53", "doc.pdf")
	relativeBackupPath := filepath.Join("backup", "doc.pdf")
	t.Setenv("DATA_BACKUP_DIR", backupDir)

	updateSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET staging_filename = $1,
    md5 = $2,
    modify_time = NOW()
WHERE file_name = $3
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
    $2,
    $3,
    '',
    $4::jsonb,
    $5
)
RETURNING id`)
	mock.ExpectQuery(insertSQL).
		WithArgs("doc.pdf", "pdf", srcPath, "[]", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(53)))

	finalizeSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET file_name = $1,
    backup_filename = $2,
    modify_time = NOW()
WHERE id = $3`)
	mock.ExpectExec(finalizeSQL).
		WithArgs(relativeHomePath, relativeBackupPath, int64(53)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := processStagingOnce(context.Background(), testLogger{}, db, stagingDir, backupDir, homeDir, nil, nil); err != nil {
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

func TestProcessStagingOnceCreatesZipParentAndChildRecords(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	tmpDir := t.TempDir()
	stagingDir := filepath.Join(tmpDir, "staging")
	backupDir := filepath.Join(tmpDir, "backup")
	homeDir := filepath.Join(tmpDir, "SemOS")
	t.Setenv("DATA_BACKUP_DIR", backupDir)
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		t.Fatalf("mkdir staging: %v", err)
	}

	srcPath := writeTempZipFile(t, stagingDir, "bundle.zip", map[string]string{
		"nested/doc.pdf": "%PDF-1.4\nhello",
	})

	updateSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET staging_filename = $1,
    md5 = $2,
    modify_time = NOW()
WHERE file_name = $3
  AND COALESCE(backup_filename, '') = ''
RETURNING id`)

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
    $2,
    $3,
    '',
    $4::jsonb,
    $5
)
RETURNING id`)

	finalizeSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET file_name = $1,
    backup_filename = $2,
    modify_time = NOW()
WHERE id = $3`)

	parentHomePath := filepath.Join(homeDir, "Artifacts", "0", "10", "bundle.zip")
	childHomePath := filepath.Join(homeDir, "Artifacts", "0", "11", "doc.pdf")

	mock.ExpectQuery(updateSQL).
		WithArgs("bundle.zip", sqlmock.AnyArg(), srcPath).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(insertSQL).
		WithArgs("bundle.zip", "zip", srcPath, "[]", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectExec(finalizeSQL).
		WithArgs(filepath.Join("Artifacts", "0", "10", "bundle.zip"), filepath.Join("backup", "bundle.zip"), int64(10)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(updateSQL).
		WithArgs("doc.pdf", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(insertSQL).
		WithArgs("doc.pdf", "pdf", sqlmock.AnyArg(), "[]", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))
	mock.ExpectExec(finalizeSQL).
		WithArgs(filepath.Join("Artifacts", "0", "11", "doc.pdf"), filepath.Join("backup", "doc.pdf"), int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := processStagingOnce(context.Background(), testLogger{}, db, stagingDir, backupDir, homeDir, nil, nil); err != nil {
		t.Fatalf("processStagingOnce: %v", err)
	}

	if _, err := os.Stat(parentHomePath); err != nil {
		t.Fatalf("expected stored zip at %q: %v", parentHomePath, err)
	}
	if _, err := os.Stat(childHomePath); err != nil {
		t.Fatalf("expected extracted child at %q: %v", childHomePath, err)
	}
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Fatalf("expected staging zip to be removed, stat err=%v", err)
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
WHERE file_name = $3
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
    $2,
    $3,
    '',
    $4::jsonb,
    $5
)
RETURNING id`)
	mock.ExpectQuery(insertSQL).
		WithArgs("doc.pdf", "pdf", srcPath, "[]", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(33)))

	recordID, updated, err := upsertStagedInputRecord(context.Background(), db, "doc.pdf", srcPath, "pdf")
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

func TestRelativePathFromHomeDir(t *testing.T) {
	homeDir := "/Users/cding/Apps/SemOS"
	got, err := relativePathFromHomeDir(homeDir, "/Users/cding/Apps/SemOS/Artifacts/0/86/std_33830.pdf")
	if err != nil {
		t.Fatalf("relativePathFromHomeDir: %v", err)
	}
	want := filepath.Join("Artifacts", "0", "86", "std_33830.pdf")
	if got != want {
		t.Fatalf("relativePathFromHomeDir = %q, want %q", got, want)
	}
}

func TestRelativePathFromParentDir(t *testing.T) {
	got, err := relativePathFromParentDir("/Users/cding/Apps/Backup", "/Users/cding/Apps/Backup/std_33830_2.pdf")
	if err != nil {
		t.Fatalf("relativePathFromParentDir: %v", err)
	}
	want := filepath.Join("Backup", "std_33830_2.pdf")
	if got != want {
		t.Fatalf("relativePathFromParentDir = %q, want %q", got, want)
	}
}

func TestUpsertStagedInputRecord_InsertsZipTypeWhenRequested(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	tmpDir := t.TempDir()
	srcPath := writeTempFile(t, tmpDir, "bundle.zip", "hello")

	updateSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET staging_filename = $1,
    md5 = $2,
    modify_time = NOW()
WHERE file_name = $3
  AND COALESCE(backup_filename, '') = ''
RETURNING id`)
	mock.ExpectQuery(updateSQL).
		WithArgs("bundle.zip", sqlmock.AnyArg(), srcPath).
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
    $2,
    $3,
    '',
    $4::jsonb,
    $5
)
RETURNING id`)
	mock.ExpectQuery(insertSQL).
		WithArgs("bundle.zip", "zip", srcPath, "[]", sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(77)))

	recordID, updated, err := upsertStagedInputRecord(context.Background(), db, "bundle.zip", srcPath, "zip")
	if err != nil {
		t.Fatalf("upsertStagedInputRecord: %v", err)
	}
	if recordID != 77 {
		t.Fatalf("expected recordID=77, got %d", recordID)
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
WHERE file_name = $3
  AND COALESCE(backup_filename, '') = ''
RETURNING id`)
	mock.ExpectQuery(updateSQL).
		WithArgs("doc.pdf", sqlmock.AnyArg(), srcPath).
		WillReturnError(fmt.Errorf("db down"))

	_, _, err = upsertStagedInputRecord(context.Background(), db, "doc.pdf", srcPath, "pdf")
	if err == nil {
		t.Fatalf("expected error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestMaxDocGoroutines_DefaultsToFour(t *testing.T) {
	t.Setenv("MAX_DOC_GOROUTINE", "")
	if got := maxDocGoroutines(); got != 4 {
		t.Errorf("maxDocGoroutines() = %d, want 4", got)
	}
}

func TestMaxDocGoroutines_ReadsEnvVar(t *testing.T) {
	t.Setenv("MAX_DOC_GOROUTINE", "8")
	if got := maxDocGoroutines(); got != 8 {
		t.Errorf("maxDocGoroutines() = %d, want 8", got)
	}
}

func TestMaxDocGoroutines_InvalidFallsBackToFour(t *testing.T) {
	t.Setenv("MAX_DOC_GOROUTINE", "bad")
	if got := maxDocGoroutines(); got != 4 {
		t.Errorf("maxDocGoroutines() = %d, want 4 for invalid input", got)
	}
}

func TestAppendParsedStatus_AddsSuccessEntry(t *testing.T) {
	start := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got, err := appendParsedStatus("[]", start, 150, nil)
	if err != nil {
		t.Fatalf("appendParsedStatus: %v", err)
	}
	if !strings.Contains(got, `"parsed"`) || !strings.Contains(got, `"success"`) {
		t.Errorf("expected parsed/success entry in %q", got)
	}
}

func TestAppendParsedStatus_AddsFailedEntry(t *testing.T) {
	start := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got, err := appendParsedStatus("[]", start, 50, errors.New("parse error"))
	if err != nil {
		t.Fatalf("appendParsedStatus: %v", err)
	}
	if !strings.Contains(got, `"failed"`) || !strings.Contains(got, "parse error") {
		t.Errorf("expected failed entry with error message in %q", got)
	}
}

func TestAppendParsedStatus_ReplacesExistingParsedEntry(t *testing.T) {
	existing := `[{"operation":"parsed","proc-status":"failed","error":"old"}]`
	start := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got, err := appendParsedStatus(existing, start, 100, nil)
	if err != nil {
		t.Fatalf("appendParsedStatus: %v", err)
	}
	if count := strings.Count(got, `"parsed"`); count != 1 {
		t.Errorf("expected exactly 1 parsed entry, got %d in %q", count, got)
	}
	if strings.Contains(got, "old") {
		t.Errorf("expected old error to be replaced in %q", got)
	}
}

func TestAppendReroutedStatus_RecordsNoteAndPaths(t *testing.T) {
	start := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got, err := appendReroutedStatus("[]", start,
		"docx: converted to pdf and routed to pdf+mineru pipeline",
		"Artifacts/0/410/doc.docx", "Artifacts/0/410/doc.pdf")
	if err != nil {
		t.Fatalf("appendReroutedStatus: %v", err)
	}
	for _, want := range []string{"docx-rerouted", "pdf+mineru", "doc.docx", "doc.pdf"} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in rerouted status %q", want, got)
		}
	}
}

func TestAppendReroutedStatus_PreservesExistingEntries(t *testing.T) {
	existing := `[{"operation":"converted","proc-status":"success"}]`
	start := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	got, err := appendReroutedStatus(existing, start, "doc: ...", "a.doc", "a.pdf")
	if err != nil {
		t.Fatalf("appendReroutedStatus: %v", err)
	}
	if !strings.Contains(got, "converted") || !strings.Contains(got, "docx-rerouted") {
		t.Errorf("expected both converted and docx-rerouted entries in %q", got)
	}
}
