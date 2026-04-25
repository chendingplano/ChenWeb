package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestUpsertStagedInputRecord_UpdatesExistingStagedRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	tmpDir := t.TempDir()
	homePath := writeTempFile(t, tmpDir, "doc.pdf", "hello")
	srcPath := "/tmp/staging/doc.pdf"
	backupPath := "/tmp/backup/doc.pdf"

	updateSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET name = $1,
    file_name = $2,
    backup_filename = $3,
    md5 = $4,
    modify_time = NOW()
WHERE file_name = $5
  AND COALESCE(backup_filename, '') = ''
RETURNING id`)

	mock.ExpectQuery(updateSQL).
		WithArgs("doc.pdf", homePath, backupPath, sqlmock.AnyArg(), srcPath).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(22)))

	updated, err := upsertStagedInputRecord(context.Background(), db, "doc.pdf", srcPath, homePath, backupPath)
	if err != nil {
		t.Fatalf("upsertStagedInputRecord: %v", err)
	}
	if !updated {
		t.Fatalf("expected updated=true")
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
	homePath := writeTempFile(t, tmpDir, "doc.pdf", "hello")
	srcPath := "/tmp/staging/doc.pdf"
	backupPath := "/tmp/backup/doc.pdf"

	updateSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET name = $1,
    file_name = $2,
    backup_filename = $3,
    md5 = $4,
    modify_time = NOW()
WHERE file_name = $5
  AND COALESCE(backup_filename, '') = ''
RETURNING id`)
	mock.ExpectQuery(updateSQL).
		WithArgs("doc.pdf", homePath, backupPath, sqlmock.AnyArg(), srcPath).
		WillReturnError(sql.ErrNoRows)

	insertSQL := regexp.QuoteMeta(`
INSERT INTO kb.inputs (
    name,
    type,
    file_name,
    backup_filename,
    status,
    md5
) VALUES (
    $1,
    'pdf',
    $2,
    $3,
    $4::jsonb,
    $5
)`)
	mock.ExpectExec(insertSQL).
		WithArgs("doc.pdf", homePath, backupPath, "[]", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	updated, err := upsertStagedInputRecord(context.Background(), db, "doc.pdf", srcPath, homePath, backupPath)
	if err != nil {
		t.Fatalf("upsertStagedInputRecord: %v", err)
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
	homePath := writeTempFile(t, tmpDir, "doc.pdf", "hello")
	srcPath := "/tmp/staging/doc.pdf"
	backupPath := "/tmp/backup/doc.pdf"

	updateSQL := regexp.QuoteMeta(`
UPDATE kb.inputs
SET name = $1,
    file_name = $2,
    backup_filename = $3,
    md5 = $4,
    modify_time = NOW()
WHERE file_name = $5
  AND COALESCE(backup_filename, '') = ''
RETURNING id`)
	mock.ExpectQuery(updateSQL).
		WithArgs("doc.pdf", homePath, backupPath, sqlmock.AnyArg(), srcPath).
		WillReturnError(fmt.Errorf("db down"))

	_, err = upsertStagedInputRecord(context.Background(), db, "doc.pdf", srcPath, homePath, backupPath)
	if err == nil {
		t.Fatalf("expected error")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
