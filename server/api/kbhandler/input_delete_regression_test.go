package kbhandler

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestPlanInputDeletePathsRefusesUnrelatedRecordFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("DATA_HOME_DIR", home)
	t.Setenv("ARTIFACT_DIR", filepath.Join(home, "Artifacts"))
	_, err := planInputDeletePaths(656, inputDeleteAssets{
		ResultFileName: filepath.Join(t.TempDir(), "656", "result.json"),
	})
	if err == nil {
		t.Fatal("accepted an unrelated folder merely because it was named 656")
	}
}

func TestArchiveInputPDFReturnsPermissionError(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "0", "656")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "a.pdf")
	writeTestFile(t, src)
	if err := os.Chmod(dir, 0o200); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0o700)
	if _, err := os.Lstat(filepath.Join(dir, "probe")); !os.IsPermission(err) {
		t.Skip("filesystem does not enforce directory permissions for this user")
	}
	if _, _, err := archiveInputPDF(root, 656, src); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected permission error, got %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source was changed: %v", err)
	}
}

func TestDeleteInputArtifactDirsRecordsPartialDeletion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "656")
	deleted := filepath.Join(dir, "a.txt")
	blocked := filepath.Join(dir, "blocked")
	retained := filepath.Join(blocked, "b.txt")
	writeTestFile(t, deleted)
	writeTestFile(t, retained)
	if err := os.Chmod(blocked, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(blocked, 0o700)
	probe := filepath.Join(blocked, "probe")
	if err := os.WriteFile(probe, nil, 0o600); !os.IsPermission(err) {
		os.Remove(probe)
		t.Skip("filesystem does not enforce directory permissions for this user")
	}
	report := newInputDeleteReport()
	deleteInputArtifactDirs([]string{dir}, &report)
	if len(report.Failures) == 0 {
		t.Fatal("expected cleanup failure")
	}
	if !slices.Contains(report.DeletedFiles, deleted) {
		t.Fatalf("deleted file missing from audit: %+v", report)
	}
	if slices.Contains(report.DeletedFiles, retained) {
		t.Fatalf("retained file reported as deleted: %+v", report)
	}
	if _, err := os.Stat(retained); err != nil {
		t.Fatalf("expected retained file: %v", err)
	}
}

func TestCleanupInputArtifactWebRecordsDeletedFilenames(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ARTIFACT_WEB_DIR", root)
	dir := filepath.Join(root, "topic")
	index := filepath.Join(dir, "metrics.txt")
	metadata := filepath.Join(dir, "metadata.txt")
	writeTestFile(t, metadata)
	writeTestFile(t, index)
	if err := os.WriteFile(index, []byte("656_metric_1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stats, failures := cleanupInputArtifactWeb(656)
	if len(failures) != 0 {
		t.Fatal(failures)
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{index, metadata} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected removal of %s: %v", path, err)
		}
		if !strings.Contains(string(encoded), path) {
			t.Fatalf("deleted path %s missing from stats: %s", path, encoded)
		}
	}
}

func expectInputDeletePendingAudit(mock sqlmock.Sqlmock, recordID int64) *sqlmock.ExpectedQuery {
	mock.ExpectQuery("SELECT current_setting\\('track_counts'\\)").
		WillReturnRows(sqlmock.NewRows([]string{"track_counts", "deleted_tables"}).AddRow(true, `[{"table":"kb.inputs","rows":1},{"table":"kb.doc_facets","rows":1},{"table":"kb.reference_artifact_links","rows":2}]`))
	return mock.ExpectQuery("INSERT INTO kb.doc_proc_logs").
		WithArgs(recordID, jsonWithFieldMatcher{field: "cleanup_status", want: "pending"}, deleteInputDocProcLogLoc).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(123))
}

func expectInputDeleteFinalAudit(mock sqlmock.Sqlmock, info sqlmock.Argument) *sqlmock.ExpectedExec {
	return mock.ExpectExec("UPDATE kb.doc_proc_logs").WithArgs(info, nil, int64(123)).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

// Require the final audit to contain actual filenames and cascade table counts.
type inputDeleteCompletedMatcher struct {
	archived string
	files    []string
}

func (m inputDeleteCompletedMatcher) Match(value driver.Value) bool {
	text, ok := value.(string)
	if !ok {
		return false
	}
	var info struct {
		Status   string              `json:"cleanup_status"`
		Archived string              `json:"archived_file"`
		Files    []string            `json:"deleted_files"`
		Tables   []inputDeletedTable `json:"deleted_tables"`
	}
	if json.Unmarshal([]byte(text), &info) != nil || info.Status != "completed" || info.Archived != m.archived {
		return false
	}
	for _, path := range m.files {
		if !slices.Contains(info.Files, path) {
			return false
		}
	}
	for _, table := range []string{"kb.inputs", "kb.doc_facets", "kb.reference_artifact_links"} {
		if !slices.ContainsFunc(info.Tables, func(item inputDeletedTable) bool { return item.Table == table && item.Rows > 0 }) {
			return false
		}
	}
	return true
}

func TestMoveFileDoesNotOverwriteDestination(t *testing.T) {
	src := filepath.Join(t.TempDir(), "a.pdf")
	dst := filepath.Join(t.TempDir(), "b.pdf")
	writeTestFile(t, src)
	if err := os.WriteFile(dst, []byte("existing archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := moveFile(src, dst); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected collision, got %v", err)
	}
	body, err := os.ReadFile(dst)
	if err != nil || string(body) != "existing archive" {
		t.Fatalf("destination overwritten: %q, %v", body, err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source lost: %v", err)
	}
}

func TestInputDeleteAuditRejectsDisabledTracking(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT current_setting").WillReturnRows(
		sqlmock.NewRows([]string{"track_counts", "tables"}).AddRow(false, "[]"))
	mock.ExpectRollback()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readInputDeletedTables(tx); err == nil {
		t.Fatal("accepted unauditable table deletions")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInputDeleteAuditRecordsPartialFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	report := newInputDeleteReport()
	report.DeletedFiles = []string{"removed.json"}
	report.Failures = []string{"remove blocked.png: permission denied"}
	mock.ExpectExec("UPDATE kb.doc_proc_logs").WithArgs(
		jsonWithFieldMatcher{field: "cleanup_status", want: "failed"}, strPtrValue(report.Failures[0]), int64(123)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := finishInputDeleteAudit(db, 123, true, report, nil, artifactWebCleanupStats{}); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
