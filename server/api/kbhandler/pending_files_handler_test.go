package kbhandler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

func TestTypeFromExtension(t *testing.T) {
	cases := map[string]string{
		"report.pdf":    "pdf",
		"REPORT.PDF":    "pdf",
		"archive.zip":   "zip",
		"notes.md":      "markdown",
		"data.xlsx":     "excel",
		"unknown.thing": "",
	}
	for name, want := range cases {
		if got := typeFromExtension(name); got != want {
			t.Errorf("typeFromExtension(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestListPendingFilesRequiresAuth(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/pending-files", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListPendingFiles(c); err != nil {
		t.Fatalf("ListPendingFiles returned error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestClaimPendingFilesRequiresAuth(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/pending-files/claim", bytes.NewBufferString(`{}`))
	req.Header.Set(echo.HeaderContentType, "application/json")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ClaimPendingFiles(c); err != nil {
		t.Fatalf("ClaimPendingFiles returned error: %v", err)
	}
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestClaimOnePendingFile_InsertsRowAndRenames(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	stagingDir := t.TempDir()
	pendingPath := filepath.Join(stagingDir, "report.pdf.pending")
	finalPath := filepath.Join(stagingDir, "report.pdf")
	if err := os.WriteFile(pendingPath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write pending file: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO kb\.inputs`).
		WithArgs(
			"tenant-alpha", int64(7), nil, "auto", "pdf", nil, nil, nil, nil, nil, nil, nil,
			"opendata", "report.pdf", finalPath, sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(99)))
	mock.ExpectCommit()

	md5Hex, err := fileMD5Hex(pendingPath)
	if err != nil {
		t.Fatalf("fileMD5Hex: %v", err)
	}

	id, err := claimOnePendingFile(db, "kb.inputs", uploadedInputInsert{
		TenantID:       "tenant-alpha",
		KSStoreID:      7,
		Type:           "pdf",
		ProcessingMode: "auto",
		ParserName:     "opendata",
		StagingName:    "report.pdf",
		StagingAbsPath: finalPath,
		MD5:            &md5Hex,
	}, pendingPath, finalPath)
	if err != nil {
		t.Fatalf("claimOnePendingFile: %v", err)
	}
	if id != 99 {
		t.Fatalf("expected id=99, got %d", id)
	}

	if _, err := os.Stat(finalPath); err != nil {
		t.Fatalf("expected renamed file to exist: %v", err)
	}
	if _, err := os.Stat(pendingPath); !os.IsNotExist(err) {
		t.Fatalf("expected pending file to be gone, err=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestClaimOnePendingFile_RollsBackWhenFileAlreadyGone(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	stagingDir := t.TempDir()
	pendingPath := filepath.Join(stagingDir, "report.pdf.pending") // never created
	finalPath := filepath.Join(stagingDir, "report.pdf")

	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO kb\.inputs`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(100)))
	mock.ExpectRollback()

	md5Hex := "deadbeef"
	_, err = claimOnePendingFile(db, "kb.inputs", uploadedInputInsert{
		TenantID:       "tenant-alpha",
		KSStoreID:      7,
		Type:           "pdf",
		ProcessingMode: "auto",
		ParserName:     "opendata",
		StagingName:    "report.pdf",
		StagingAbsPath: finalPath,
		MD5:            &md5Hex,
	}, pendingPath, finalPath)
	if err == nil {
		t.Fatalf("expected error when pending file is already gone")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
