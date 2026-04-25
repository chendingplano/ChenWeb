package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newUpdateInputContext(t *testing.T, id string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/kb/inputs/"+id, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/inputs/:id")
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func TestUpdateInputSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	expectResolveInputTablePlural(mock)
	updateQuery := regexp.QuoteMeta("UPDATE kb.inputs SET modify_time = NOW(), authors = $1, doc_metadata = $2, title = $3 WHERE id = $4")
	mock.ExpectExec(updateQuery).
		WithArgs(`["Alice","Bob"]`, `{"doc_no":"GB/T 50378-2019"}`, "Updated Title", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	expectResolveNameColumnStaging(mock)
	expectResolveParserNameColumn(mock, true)
	selectQuery := regexp.QuoteMeta(`
SELECT
    i.id, i.staging_filename AS name, COALESCE(i.parser_name, '') AS parser_name, i.type, i.tenant_id, i.ks_store_id, i.title, i.doc_no, i.ks_desc, i.source,
    i.file_name, i.backup_filename, i.result_filename, i.publish_date,
    i.authors, i.owner, COALESCE(i.status, '[]'::jsonb) AS status,
    i.create_time, i.modify_time, i.public_info, i.private_info, i.doc_metadata::text,
    i.notes, i.error_msg
FROM kb.inputs i
WHERE i.id = $1
`)
	rows := sqlmock.NewRows([]string{
		"id", "name", "parser_name", "type", "tenant_id", "ks_store_id", "title", "doc_no", "ks_desc", "source", "file_name",
		"backup_filename", "result_filename", "publish_date", "authors", "owner",
		"status", "create_time", "modify_time", "public_info", "private_info", "doc_metadata",
		"notes", "error_msg",
	}).AddRow(
		int64(7), "input_7.pdf", "mineru", "pdf", "tenant-alpha", int64(9), "Updated Title", "GB/T 50378-2019", "Store desc", "upload",
		"/tmp/input_7.pdf", "/tmp/input_7.bak", "/tmp/input_7.json", time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
		`["Alice","Bob"]`, int64(9),
		`[]`, time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC), time.Date(2026, 4, 18, 10, 30, 0, 0, time.UTC),
		`{"visibility":"public"}`, `{"internal":"yes"}`, `{"doc_no":"GB/T 50378-2019"}`,
		"note", "",
	)
	mock.ExpectQuery(selectQuery).WithArgs(int64(7)).WillReturnRows(rows)

	c, rec := newUpdateInputContext(t, "7", `{
		"title":"Updated Title",
		"authors":["Alice","Bob"],
		"doc_metadata":{"doc_no":"GB/T 50378-2019"}
	}`)
	if err := UpdateInput(c); err != nil {
		t.Fatalf("UpdateInput returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload inputDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true")
	}
	if payload.Record.ID != 7 {
		t.Fatalf("expected id=7, got %d", payload.Record.ID)
	}
	if payload.Record.Title == nil || *payload.Record.Title != "Updated Title" {
		t.Fatalf("expected updated title, got %+v", payload.Record.Title)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateInputInvalidPublishDate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	expectResolveInputTablePlural(mock)

	c, rec := newUpdateInputContext(t, "9", `{"publish_date":"not-a-date"}`)
	if err := UpdateInput(c); err != nil {
		t.Fatalf("UpdateInput returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateInputNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	expectResolveInputTablePlural(mock)
	updateQuery := regexp.QuoteMeta("UPDATE kb.inputs SET modify_time = NOW(), title = $1 WHERE id = $2")
	mock.ExpectExec(updateQuery).
		WithArgs("No Such Record", int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	c, rec := newUpdateInputContext(t, "999", `{"title":"No Such Record"}`)
	if err := UpdateInput(c); err != nil {
		t.Fatalf("UpdateInput returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
