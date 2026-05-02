package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestReadSummaryCategoryRecords(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	artifactDir := t.TempDir()
	summaryTreeDir := t.TempDir()

	t.Setenv("ARTIFACT_DIR", artifactDir)

	expectResolveInputTablePlural(mock)
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "staging_filename").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "parser_name").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	recordID := int64(1042)
	query := regexp.QuoteMeta(`SELECT COALESCE(i.staging_filename, '') AS staging_filename, COALESCE(i.parser_name, '') AS parser_name, i.file_name FROM kb.inputs i WHERE i.id = $1`)
	mock.ExpectQuery(query).
		WithArgs(recordID).
		WillReturnRows(sqlmock.NewRows([]string{"staging_filename", "parser_name", "file_name"}).
			AddRow("sample.pdf", "pdfplumber", "/tmp/standards/sample.pdf"))

	mustWriteFile(t, filepath.Join(summaryTreeDir, "finance", "tax", "summaries.txt"), "1042_1_0001\n")
	mustWriteFile(t, filepath.Join(artifactDir, "1", "1042", "summary_1_0001.txt"), `summary_id: "1042_1_0001"
record_id: 1042
level: 1
lines: ["7-9"]
children: ["1042_0_0001", "1042_0_0002"]
keywords: ["tax", "filing"]
category_paths: ["finance/tax"]
summary_begin:
Tax filing summary text
summary_end`)
	mustWriteFile(t, filepath.Join(artifactDir, "1", "1042", "sample_pdfplumber.corrected"), "1\t1\tparagraph\tunchanged\tTimes\t10\t[0,0,1,1]\tA\n7\t3\tparagraph\tunchanged\tTimes\t10\t[0,0,1,1]\tB\n9\t3\tparagraph\tunchanged\tTimes\t10\t[0,0,1,1]\tC\n")

	results, err := readSummaryCategoryRecords(summaryTreeDir, "finance/tax")
	if err != nil {
		t.Fatalf("readSummaryCategoryRecords: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 record, got %d", len(results))
	}
	if results[0].ID != "1042_1_0001" {
		t.Fatalf("unexpected summary id: %+v", results[0])
	}
	if results[0].PdfFileName != "sample.pdf" {
		t.Fatalf("unexpected file name: %+v", results[0])
	}
	if results[0].Page != 3 {
		t.Fatalf("unexpected page: %+v", results[0])
	}
	if results[0].SummaryText != "Tax filing summary text" {
		t.Fatalf("unexpected summary text: %+v", results[0])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestReadSummaryCategoryRecordsMissingSummariesFile(t *testing.T) {
	artifactDir := t.TempDir()
	summaryTreeDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)

	results, err := readSummaryCategoryRecords(summaryTreeDir, "medical_standards/surgery")
	if err != nil {
		t.Fatalf("readSummaryCategoryRecords should allow missing summaries.txt: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no summaries, got %+v", results)
	}
}

func TestGetSummaryCategorySuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	artifactDir := t.TempDir()
	summaryTreeDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	t.Setenv("SUMMARY_TREE_DIR", summaryTreeDir)

	expectResolveInputTablePlural(mock)
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "staging_filename").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "parser_name").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	query := regexp.QuoteMeta(`SELECT COALESCE(i.staging_filename, '') AS staging_filename, COALESCE(i.parser_name, '') AS parser_name, i.file_name FROM kb.inputs i WHERE i.id = $1`)
	mock.ExpectQuery(query).
		WithArgs(int64(1042)).
		WillReturnRows(sqlmock.NewRows([]string{"staging_filename", "parser_name", "file_name"}).
			AddRow("sample.pdf", "pdfplumber", "/tmp/standards/sample.pdf"))

	mustWriteFile(t, filepath.Join(summaryTreeDir, "finance", "tax", "summaries.txt"), "1042_1_0001\n")
	mustWriteFile(t, filepath.Join(artifactDir, "1", "1042", "summary_1_0001.txt"), `summary_id: "1042_1_0001"
record_id: 1042
level: 1
lines: ["7-9"]
children: ["1042_0_0001"]
keywords: ["tax"]
category_paths: ["finance/tax"]
summary_begin:
Tax filing summary text
summary_end`)
	mustWriteFile(t, filepath.Join(artifactDir, "1", "1042", "sample_pdfplumber.corrected"), "7\t3\tparagraph\tunchanged\tTimes\t10\t[0,0,1,1]\tB\n")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/summary-category?category_path=finance/tax", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetSummaryCategory(c); err != nil {
		t.Fatalf("GetSummaryCategory returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Status       bool                    `json:"status"`
		CategoryPath string                  `json:"categoryPath"`
		Summaries    []summaryCategoryRecord `json:"summaries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Status || payload.CategoryPath != "finance/tax" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if len(payload.Summaries) != 1 || payload.Summaries[0].Page != 3 {
		t.Fatalf("unexpected summaries: %+v", payload.Summaries)
	}
}

func TestGetSummaryCategoryMissingSummariesFile(t *testing.T) {
	artifactDir := t.TempDir()
	summaryTreeDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)
	t.Setenv("SUMMARY_TREE_DIR", summaryTreeDir)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/summary-category?category_path=medical_standards/surgery", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetSummaryCategory(c); err != nil {
		t.Fatalf("GetSummaryCategory returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Status       bool                    `json:"status"`
		CategoryPath string                  `json:"categoryPath"`
		Summaries    []summaryCategoryRecord `json:"summaries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Status || payload.CategoryPath != "medical_standards/surgery" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if len(payload.Summaries) != 0 {
		t.Fatalf("expected empty summaries, got %+v", payload.Summaries)
	}
}
