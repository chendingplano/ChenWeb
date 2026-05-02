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

func TestReadRecordSummaryCards(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	artifactDir := t.TempDir()
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

	mustWriteFile(t, filepath.Join(artifactDir, "1", "1042", "summary_1_0002.txt"), `summary_id: "1042_1_0002"
record_id: 1042
level: 1
lines: ["12-13"]
keywords: ["surgery", "post-op"]
summary_begin:
Post-op summary text
summary_end`)
	mustWriteFile(t, filepath.Join(artifactDir, "1", "1042", "summary_1_0001.txt"), `summary_id: "1042_1_0001"
record_id: 1042
level: 1
lines: ["7-9"]
keywords: ["surgery", "risk"]
summary_begin:
Primary summary text
summary_end`)
	mustWriteFile(t, filepath.Join(artifactDir, "1", "1042", "sample_pdfplumber.corrected"), "7\t3\tparagraph\tunchanged\tTimes\t10\t[0,0,1,1]\tB\n12\t5\tparagraph\tunchanged\tTimes\t10\t[0,0,1,1]\tC\n")

	results, err := readRecordSummaryCards(nil, recordID)
	if err != nil {
		t.Fatalf("readRecordSummaryCards: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 records, got %d", len(results))
	}
	if results[0].ID != "1042_1_0001" || results[0].Page != 3 {
		t.Fatalf("unexpected first record: %+v", results[0])
	}
	if results[1].ID != "1042_1_0002" || results[1].Page != 5 {
		t.Fatalf("unexpected second record: %+v", results[1])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestReadRecordSummaryCardsMissingDir(t *testing.T) {
	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)

	results, err := readRecordSummaryCards(nil, 1042)
	if err != nil {
		t.Fatalf("readRecordSummaryCards should allow missing record dir: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no summaries, got %+v", results)
	}
}

func TestReadRecordSummaryCardsFindsRecordInNonDefaultGroupDir(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)

	expectResolveInputTablePlural(mock)
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "staging_filename").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT EXISTS \(`).
		WithArgs("kb", "inputs", "parser_name").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	recordID := int64(93)
	query := regexp.QuoteMeta(`SELECT COALESCE(i.staging_filename, '') AS staging_filename, COALESCE(i.parser_name, '') AS parser_name, i.file_name FROM kb.inputs i WHERE i.id = $1`)
	mock.ExpectQuery(query).
		WithArgs(recordID).
		WillReturnRows(sqlmock.NewRows([]string{"staging_filename", "parser_name", "file_name"}).
			AddRow("sample.pdf", "pdfplumber", "/tmp/standards/sample.pdf"))

	mustWriteFile(t, filepath.Join(artifactDir, "7", "93", "summary_0_0001.txt"), `summary_id: "93_0_0001"
record_id: 93
level: 0
lines: ["1-2"]
keywords: ["health"]
summary_begin:
Leaf summary text
summary_end`)
	mustWriteFile(t, filepath.Join(artifactDir, "7", "93", "sample_pdfplumber.corrected"), "1\t9\tparagraph\tunchanged\tTimes\t10\t[0,0,1,1]\tA\n")

	results, err := readRecordSummaryCards(nil, recordID)
	if err != nil {
		t.Fatalf("readRecordSummaryCards: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 record, got %d", len(results))
	}
	if results[0].ID != "93_0_0001" || results[0].Page != 9 {
		t.Fatalf("unexpected record: %+v", results[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestGetRecordSummariesSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	artifactDir := t.TempDir()
	t.Setenv("ARTIFACT_DIR", artifactDir)

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

	mustWriteFile(t, filepath.Join(artifactDir, "1", "1042", "summary_1_0001.txt"), `summary_id: "1042_1_0001"
record_id: 1042
level: 1
lines: ["7-9"]
keywords: ["surgery", "risk"]
summary_begin:
Primary summary text
summary_end`)
	mustWriteFile(t, filepath.Join(artifactDir, "1", "1042", "sample_pdfplumber.corrected"), "7\t3\tparagraph\tunchanged\tTimes\t10\t[0,0,1,1]\tB\n")

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/record-summaries?record_id=1042", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetRecordSummaries(c); err != nil {
		t.Fatalf("GetRecordSummaries returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Status    bool                    `json:"status"`
		RecordID  int64                   `json:"recordId"`
		Summaries []summaryCategoryRecord `json:"summaries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Status || payload.RecordID != 1042 || len(payload.Summaries) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}
