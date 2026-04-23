package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func TestParseCorrectedLine(t *testing.T) {
	got, ok := parseCorrectedLine("12\t3\tparagraph\theading-2\tTimes-Roman\t10\t[11.5, 22, 100, 130.25]\tSection heading")
	if !ok {
		t.Fatalf("expected parse success")
	}
	if got.LineNumber != 12 || got.PageNumber != 3 {
		t.Fatalf("unexpected line/page: %+v", got)
	}
	if got.CorrectedLineType != "heading-2" {
		t.Fatalf("unexpected corrected type: %q", got.CorrectedLineType)
	}
	if len(got.Coords) != 4 {
		t.Fatalf("expected 4 coords, got %v", got.Coords)
	}
}

func TestParseCorrectedLineRejectsInvalid(t *testing.T) {
	if _, ok := parseCorrectedLine("1\t1\tparagraph\tunchanged\tfont\t11\tbad\tcontent"); ok {
		t.Fatalf("expected parse failure")
	}
	if _, ok := parseCorrectedLine("1\t1\tparagraph"); ok {
		t.Fatalf("expected parse failure")
	}
}

func TestGetDocStructureSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	tmp := t.TempDir()
	t.Setenv("ARTIFACT_DIR", tmp)

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
			AddRow("sample.pdf", "pdfplumber", "/tmp/sample.pdf"))

	groupID := recordID / 1000
	correctedDir := filepath.Join(tmp, "1", "1042")
	if groupID != 1 {
		t.Fatalf("unexpected group id: %d", groupID)
	}
	if err := os.MkdirAll(correctedDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	correctedPath := filepath.Join(correctedDir, "sample_pdfplumber.corrected")
	content := "1\t1\tparagraph\theading-1\tTimes-Roman\t12\t[10,20,30,40]\tIntro\n2\t1\tlist-item\tunchanged\tTimes-Roman\t11\t[12,22,32,42]\tPoint"
	if err := os.WriteFile(correctedPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write corrected file: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/doc-structure?input_record_id=1042", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := GetDocStructure(c); err != nil {
		t.Fatalf("GetDocStructure returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload docStructureResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Status || payload.InputID != recordID {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Total != 2 || payload.Pages != 1 {
		t.Fatalf("unexpected counts: total=%d pages=%d", payload.Total, payload.Pages)
	}
	if filepath.Base(payload.CorrectedFile) != "sample_pdfplumber.corrected" {
		t.Fatalf("unexpected corrected file: %s", payload.CorrectedFile)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
