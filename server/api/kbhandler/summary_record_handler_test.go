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
	if len(results[0].Coords) != 4 || results[0].Coords[2] != 1 {
		t.Fatalf("unexpected first coords: %+v", results[0])
	}
	if len(results[0].Targets) != 1 || results[0].Targets[0].Page != 3 {
		t.Fatalf("unexpected first targets: %+v", results[0].Targets)
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
	if len(results[0].Coords) != 4 {
		t.Fatalf("unexpected coords: %+v", results[0])
	}
	if len(results[0].Targets) != 2 || results[0].Targets[0].Page != 9 {
		t.Fatalf("unexpected targets: %+v", results[0].Targets)
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
	if len(payload.Summaries[0].Coords) != 4 {
		t.Fatalf("unexpected coords in payload: %+v", payload.Summaries[0])
	}
	if len(payload.Summaries[0].Targets) != 1 {
		t.Fatalf("unexpected targets in payload: %+v", payload.Summaries[0])
	}
}

func TestReadRecordSummaryCards_AllowsMissingCorrectedArtifact(t *testing.T) {
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

	recordID := int64(99)
	query := regexp.QuoteMeta(`SELECT COALESCE(i.staging_filename, '') AS staging_filename, COALESCE(i.parser_name, '') AS parser_name, i.file_name FROM kb.inputs i WHERE i.id = $1`)
	mock.ExpectQuery(query).
		WithArgs(recordID).
		WillReturnRows(sqlmock.NewRows([]string{"staging_filename", "parser_name", "file_name"}).
			AddRow("std_20039.pdf", "opendata", "/tmp/standards/std_20039.pdf"))

	mustWriteFile(t, filepath.Join(artifactDir, "0", "99", "summary_0_0001.txt"), `summary_id: "99_0_0001"
record_id: 99
level: 0
lines: ["1-3"]
keywords: ["scope", "definitions"]
summary_begin:
Scope summary
summary_end`)

	results, err := readRecordSummaryCards(nil, recordID)
	if err != nil {
		t.Fatalf("readRecordSummaryCards: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(results))
	}
	if results[0].Page != 1 {
		t.Fatalf("expected default page 1 without corrected artifact, got %+v", results[0])
	}
	if len(results[0].Coords) != 0 {
		t.Fatalf("expected empty coords without corrected artifact, got %+v", results[0])
	}
	if len(results[0].Targets) != 0 {
		t.Fatalf("expected empty targets without corrected artifact, got %+v", results[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestExpandSummaryTargetsMergesContinuousLinesAndSkipsErrorImage(t *testing.T) {
	targets := expandSummaryTargets([]string{"76-88"}, map[int]summaryLineTarget{
		76: {page: 5, lineType: "paragraph", coords: []float64{0, 389.462, 623.999, 478.248}},
		77: {page: 5, lineType: "paragraph", coords: []float64{83.76, 436.27, 140.4, 447.37}},
		78: {page: 5, lineType: "paragraph", coords: []float64{83.86, 421.006, 99.898, 433.058}},
		79: {page: 5, lineType: "paragraph", coords: []float64{105.6, 405.503, 427.022, 417.109}},
		80: {page: 5, lineType: "paragraph", coords: []float64{105.36, 390.37, 546.721, 401.77}},
		81: {page: 5, lineType: "paragraph", coords: []float64{84, 375.3, 297.121, 386.4}},
		82: {page: 5, lineType: "heading-2", coords: []float64{85.08, 192.43, 298.081, 235.039}},
		83: {page: 5, lineType: "heading-2", coords: []float64{85.56, 146.35, 548.164, 188.869}},
		84: {page: 5, lineType: "paragraph", coords: []float64{86.4, 131.17, 546.961, 142.57}},
		85: {page: 5, lineType: "paragraph", coords: []float64{85.68, 116.1, 199.44, 127.2}},
		86: {page: 5, lineType: "heading-2", coords: []float64{85.81, 85.573, 227.043, 112.098}},
		87: {page: 6, lineType: "image", coords: []float64{0, 0, 624, 879.12}},
		88: {page: 6, lineType: "paragraph", coords: []float64{89.76, 747.55, 553.2, 773.53}},
	})

	if len(targets) != 2 {
		t.Fatalf("expected 2 merged targets, got %+v", targets)
	}
	if targets[0].Page != 5 || len(targets[0].Coords) != 4 {
		t.Fatalf("unexpected first merged target: %+v", targets[0])
	}
	if targets[0].Coords[0] == 0 || targets[0].Coords[2] >= 600 {
		t.Fatalf("expected watermark/error boxes to be ignored, got %+v", targets[0])
	}
	if targets[1].Page != 6 || len(targets[1].Coords) != 4 {
		t.Fatalf("unexpected second merged target: %+v", targets[1])
	}
}
