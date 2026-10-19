package kbhandler

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func writeTestFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestPlanInputDeletePathsRefusesFolderNotNamedAfterRecord(t *testing.T) {
	dataHome := t.TempDir()
	t.Setenv("DATA_HOME_DIR", dataHome)
	t.Setenv("ARTIFACT_DIR", filepath.Join(dataHome, "Artifacts"))

	// The result file sits directly in the data home, so its folder is the data home itself.
	_, err := planInputDeletePaths(656, inputDeleteAssets{
		FileName:       "Artifacts/0/656/a.pdf",
		ResultFileName: "result.json",
	})
	if err == nil || !strings.Contains(err.Error(), "refusing to remove folder") {
		t.Fatalf("expected refusal for a shared folder, got %v", err)
	}
}

func TestPlanInputDeletePathsRequiresArtifactDir(t *testing.T) {
	t.Setenv("DATA_HOME_DIR", t.TempDir())
	t.Setenv("ARTIFACT_DIR", "")

	_, err := planInputDeletePaths(656, inputDeleteAssets{FileName: "Artifacts/0/656/a.pdf"})
	if err == nil || !strings.Contains(err.Error(), "ARTIFACT_DIR is not set") {
		t.Fatalf("expected ARTIFACT_DIR error, got %v", err)
	}
}

func TestArchiveInputPDFMovesFileAndAvoidsNameCollision(t *testing.T) {
	archiveRoot := t.TempDir()
	srcDir := t.TempDir()

	first := filepath.Join(srcDir, "a.pdf")
	writeTestFile(t, first)
	dst, missing, err := archiveInputPDF(archiveRoot, 656, first)
	if err != nil || missing {
		t.Fatalf("archive failed: missing=%v err=%v", missing, err)
	}
	if want := filepath.Join(archiveRoot, "0", "656", "a.pdf"); dst != want {
		t.Fatalf("archived to %s, want %s", dst, want)
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("expected source to be moved away, stat error: %v", err)
	}

	second := filepath.Join(srcDir, "a.pdf")
	writeTestFile(t, second)
	dst2, _, err := archiveInputPDF(archiveRoot, 656, second)
	if err != nil {
		t.Fatalf("second archive failed: %v", err)
	}
	if want := filepath.Join(archiveRoot, "0", "656", "a_1.pdf"); dst2 != want {
		t.Fatalf("second archive went to %s, want %s", dst2, want)
	}
}

func TestArchiveInputPDFReportsMissingSourceWithoutError(t *testing.T) {
	dst, missing, err := archiveInputPDF(t.TempDir(), 656, filepath.Join(t.TempDir(), "gone.pdf"))
	if err != nil {
		t.Fatalf("expected no error for a missing PDF, got %v", err)
	}
	if !missing || dst != "" {
		t.Fatalf("expected missing=true and no archive path, got missing=%v dst=%q", missing, dst)
	}
}

func TestDeleteInputFilesReportsFailures(t *testing.T) {
	// A non-empty directory cannot be removed with os.Remove, so this must be reported as a failure.
	stuck := filepath.Join(t.TempDir(), "stuck")
	writeTestFile(t, filepath.Join(stuck, "child.txt"))

	report := newInputDeleteReport()
	deleteInputFiles([]string{stuck}, &report)
	if len(report.Failures) != 1 {
		t.Fatalf("expected one reported failure, got %v", report.Failures)
	}
	if len(report.DeletedFiles) != 0 {
		t.Fatalf("expected nothing reported as deleted, got %v", report.DeletedFiles)
	}
}

func TestDeleteInputWithFilesRequiresArchiveDir(t *testing.T) {
	t.Setenv("ARCHIVE_DIR", "")

	c, rec := newDeleteInputContext(t, "656")
	c.QueryParams().Set("delete_file", "true")
	if err := DeleteInput(c); err != nil {
		t.Fatalf("DeleteInput returned error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "CWB_KB_M_421") {
		t.Fatalf("expected ARCHIVE_DIR error code in body, got %s", rec.Body.String())
	}
}

func TestDeleteInputWithFilesArchivesPDFAndDeletesArtifacts(t *testing.T) {
	for _, scenario := range []struct {
		name                         string
		insertFailure, updateFailure bool
	}{
		{name: "success"}, {name: "audit_insert_failure", insertFailure: true}, {name: "audit_update_failure", updateFailure: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {

			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock.New failed: %v", err)
			}
			defer db.Close()

			oldDB := ApiTypes.ProjectDBHandle
			ApiTypes.ProjectDBHandle = db
			defer func() { ApiTypes.ProjectDBHandle = oldDB }()

			dataHome := t.TempDir()
			archiveRoot := t.TempDir()
			t.Setenv("DATA_HOME_DIR", dataHome)
			t.Setenv("ARTIFACT_DIR", filepath.Join(dataHome, "Artifacts"))
			t.Setenv("ARCHIVE_DIR", archiveRoot)

			recordDir := filepath.Join(dataHome, "Artifacts", "0", "430")
			pdf := filepath.Join(recordDir, "a.pdf")
			result := filepath.Join(recordDir, "a_mineru.json")
			image := filepath.Join(recordDir, "images", "p1.png")
			for _, path := range []string{pdf, result, image} {
				writeTestFile(t, path)
			}
			archived := filepath.Join(archiveRoot, "0", "430", "a.pdf")

			expectResolveInputTablePlural(mock)
			mock.ExpectBegin()
			selectQuery := regexp.QuoteMeta(`SELECT COALESCE(file_name, ''), COALESCE(backup_filename, ''), COALESCE(result_filename, '') FROM kb.inputs WHERE id = $1`)
			mock.ExpectQuery(selectQuery).
				WithArgs(int64(430)).
				WillReturnRows(sqlmock.NewRows([]string{"file_name", "backup_filename", "result_filename"}).
					AddRow("Artifacts/0/430/a.pdf", "", "Artifacts/0/430/a_mineru.json"))

			expectDocProcRunLogCleanup(mock, true)
			expectInputReferenceCleanup(mock, 430)
			for _, spec := range inputRelatedDeleteSpecs {
				expectInputRelatedColumnCheck(mock, spec, true)
				stmt := "DELETE FROM " + spec.Table + " WHERE " + spec.Column + " = $1"
				if spec.Table == "kb.doc_proc_logs" {
					stmt += " AND entry_type <> 'delete_input'"
				}
				mock.ExpectExec(regexp.QuoteMeta(stmt)).
					WithArgs(int64(430)).
					WillReturnResult(sqlmock.NewResult(0, 0))
			}
			mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.inputs WHERE id = $1`)).
				WithArgs(int64(430)).
				WillReturnResult(sqlmock.NewResult(0, 1))
			pending := expectInputDeletePendingAudit(mock, 430)
			if scenario.insertFailure {
				pending.WillReturnError(os.ErrPermission)
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
				final := expectInputDeleteFinalAudit(mock, inputDeleteCompletedMatcher{archived: archived, files: []string{result, image}})
				if scenario.updateFailure {
					final.WillReturnError(os.ErrPermission)
				}
			}

			c, rec := newDeleteInputContext(t, "430")
			c.QueryParams().Set("delete_file", "true")
			if err := DeleteInput(c); err != nil {
				t.Fatalf("DeleteInput returned error: %v", err)
			}
			expectedStatus := http.StatusOK
			if scenario.insertFailure || scenario.updateFailure {
				expectedStatus = http.StatusInternalServerError
			}
			if rec.Code != expectedStatus {
				t.Fatalf("expected %d, got %d, body=%s", expectedStatus, rec.Code, rec.Body.String())
			}
			if scenario.insertFailure {
				for _, path := range []string{pdf, result, image} {
					if _, err := os.Stat(path); err != nil {
						t.Fatalf("file changed before durable audit: %s: %v", path, err)
					}
				}
				if _, err := os.Stat(archived); !os.IsNotExist(err) {
					t.Fatalf("PDF was not restored: %v", err)
				}
				if err := mock.ExpectationsWereMet(); err != nil {
					t.Fatal(err)
				}
				return
			}
			if scenario.updateFailure && !strings.Contains(rec.Body.String(), "cleanup remains pending") {
				t.Fatalf("missing pending audit notice: %s", rec.Body.String())
			}

			if _, err := os.Stat(archived); err != nil {
				t.Fatalf("expected PDF archived to %s: %v", archived, err)
			}
			for _, path := range []string{pdf, result, image, recordDir} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("expected %s to be removed, stat error: %v", path, err)
				}
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatalf("unmet db expectations: %v", err)
			}

		})
	}
}

func TestDeleteInputWithFilesRestoresPDFWhenDatabaseDeleteFails(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	dataHome := t.TempDir()
	archiveRoot := t.TempDir()
	t.Setenv("DATA_HOME_DIR", dataHome)
	t.Setenv("ARTIFACT_DIR", filepath.Join(dataHome, "Artifacts"))
	t.Setenv("ARCHIVE_DIR", archiveRoot)

	pdf := filepath.Join(dataHome, "Artifacts", "0", "430", "a.pdf")
	writeTestFile(t, pdf)

	expectResolveInputTablePlural(mock)
	mock.ExpectBegin()
	selectQuery := regexp.QuoteMeta(`SELECT COALESCE(file_name, ''), COALESCE(backup_filename, ''), COALESCE(result_filename, '') FROM kb.inputs WHERE id = $1`)
	mock.ExpectQuery(selectQuery).
		WithArgs(int64(430)).
		WillReturnRows(sqlmock.NewRows([]string{"file_name", "backup_filename", "result_filename"}).
			AddRow("Artifacts/0/430/a.pdf", "", ""))
	expectDocProcRunLogCleanup(mock, false)
	expectInputRelatedColumnCheck(mock, inputRelatedDeleteSpec{Table: "kb.ref_mentions", Column: "input_record_id"}, false)
	for _, spec := range inputRelatedDeleteSpecs {
		expectInputRelatedColumnCheck(mock, spec, false)
	}
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.inputs WHERE id = $1`)).
		WithArgs(int64(430)).
		WillReturnError(os.ErrPermission)
	mock.ExpectRollback()

	c, rec := newDeleteInputContext(t, "430")
	c.QueryParams().Set("delete_file", "true")
	if err := DeleteInput(c); err != nil {
		t.Fatalf("DeleteInput returned error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "the PDF was restored") {
		t.Fatalf("expected restore notice in body, got %s", rec.Body.String())
	}
	if _, err := os.Stat(pdf); err != nil {
		t.Fatalf("expected PDF restored to %s: %v", pdf, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
