package kbhandler

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/labstack/echo/v4"
)

var dpFixedTime = time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)

func newDocProcessorNameContext(t *testing.T, method, name, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	target := "/api/v1/kb/doc-processors/" + name
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/doc-processors/:name")
	c.SetParamNames("name")
	c.SetParamValues(name)
	return c, rec
}

const dpListQuery = "SELECT" + docProcessorColumns + "\nORDER BY name_as_id"

const dpSearchQuery = "SELECT" + docProcessorColumns + "\nWHERE name_as_id ILIKE $1 OR display_name ILIKE $1\nORDER BY name_as_id"

const dpFetchQuery = "SELECT" + docProcessorColumns + "\nWHERE name_as_id = $1"

const dpExistsQuery = `SELECT EXISTS(SELECT 1 FROM kb.doc_processors WHERE name_as_id = $1)`

const dpInsertQuery = `
INSERT INTO kb.doc_processors (name_as_id, display_name, description, type, require_llm, status, notes)
VALUES ($1, $2, $3, $4, $5, $6, $7)`

const dpDeleteQuery = `DELETE FROM kb.doc_processors WHERE name_as_id = $1`

var dpCols = []string{
	"name_as_id", "display_name", "description", "type", "require_llm",
	"status", "notes", "create_time", "modify_time",
}

func dpRow(name, display string, desc any, typ string, llm bool, status string, notes any) []driver.Value {
	return []driver.Value{name, display, desc, typ, llm, status, notes, dpFixedTime, dpFixedTime}
}

func expectFetchDocProcessor(mock sqlmock.Sqlmock, name string, row []driver.Value) {
	mock.ExpectQuery(regexp.QuoteMeta(dpFetchQuery)).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(dpCols).AddRow(row...))
}

func TestListDocProcessorsSuccess(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectQuery(regexp.QuoteMeta(dpListQuery)).
		WillReturnRows(sqlmock.NewRows(dpCols).
			AddRow(dpRow("blocking", "Blocking Processor", "Breaks blocks", "mandatory", false, "active", "seqno 1")...).
			AddRow(dpRow("extract_metrics", "Extract Metrics", nil, "configurable", true, "suspended", nil)...))

	c, rec := newPipelineContext(t, http.MethodGet, "/api/v1/kb/doc-processors", "")
	if err := ListDocProcessors(c); err != nil {
		t.Fatalf("ListDocProcessors returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload listDocProcessorsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || len(payload.Results) != 2 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Results[0].NameAsID != "blocking" || payload.Results[0].Type != "mandatory" || payload.Results[0].RequireLLM {
		t.Fatalf("unexpected first result: %+v", payload.Results[0])
	}
	if payload.Results[1].Description != nil || payload.Results[1].Notes != nil || payload.Results[1].Status != "suspended" {
		t.Fatalf("unexpected second result: %+v", payload.Results[1])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListDocProcessorsSearch(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectQuery(regexp.QuoteMeta(dpSearchQuery)).
		WithArgs("%block%").
		WillReturnRows(sqlmock.NewRows(dpCols).
			AddRow(dpRow("blocking", "Blocking Processor", nil, "mandatory", false, "active", nil)...))

	c, rec := newPipelineContext(t, http.MethodGet, "/api/v1/kb/doc-processors?search=block", "")
	if err := ListDocProcessors(c); err != nil {
		t.Fatalf("ListDocProcessors returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload listDocProcessorsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if len(payload.Results) != 1 || payload.Results[0].NameAsID != "blocking" {
		t.Fatalf("unexpected search results: %+v", payload.Results)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreateDocProcessorSuccess(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectQuery(regexp.QuoteMeta(dpExistsQuery)).WithArgs("extract_metrics").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(regexp.QuoteMeta(dpInsertQuery)).
		WithArgs("extract_metrics", "Extract Metrics", "Extracts metrics", "configurable", true, "active", "note").
		WillReturnResult(sqlmock.NewResult(1, 1))
	expectFetchDocProcessor(mock, "extract_metrics", dpRow("extract_metrics", "Extract Metrics", "Extracts metrics", "configurable", true, "active", "note"))

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-processors", `{
		"name_as_id":"extract_metrics",
		"display_name":"Extract Metrics",
		"description":"Extracts metrics",
		"type":"configurable",
		"require_llm":true,
		"status":"active",
		"notes":"note"
	}`)
	if err := CreateDocProcessor(c); err != nil {
		t.Fatalf("CreateDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload docProcessorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || payload.Record.NameAsID != "extract_metrics" || payload.Record.RequireLLM != true {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreateDocProcessorDefaultsStatusActive(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectQuery(regexp.QuoteMeta(dpExistsQuery)).WithArgs("chunking").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	// description/notes omitted → NULL args; status omitted → defaults to 'active'.
	mock.ExpectExec(regexp.QuoteMeta(dpInsertQuery)).
		WithArgs("chunking", "Chunking", nil, "mandatory", false, "active", nil).
		WillReturnResult(sqlmock.NewResult(2, 1))
	expectFetchDocProcessor(mock, "chunking", dpRow("chunking", "Chunking", nil, "mandatory", false, "active", nil))

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-processors", `{
		"name_as_id":"chunking",
		"display_name":"Chunking",
		"type":"mandatory"
	}`)
	if err := CreateDocProcessor(c); err != nil {
		t.Fatalf("CreateDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreateDocProcessorMissingName(t *testing.T) {
	_, mock := installPolicyDB(t)

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-processors", `{
		"display_name":"No Name"
	}`)
	if err := CreateDocProcessor(c); err != nil {
		t.Fatalf("CreateDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreateDocProcessorDuplicateName(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectQuery(regexp.QuoteMeta(dpExistsQuery)).WithArgs("blocking").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-processors", `{
		"name_as_id":"blocking",
		"display_name":"Blocking Processor",
		"type":"mandatory"
	}`)
	if err := CreateDocProcessor(c); err != nil {
		t.Fatalf("CreateDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already exists") {
		t.Fatalf("expected duplicate-name message, got: %s", rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreateDocProcessorInvalidTypeRejected(t *testing.T) {
	_, mock := installPolicyDB(t)

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-processors", `{
		"name_as_id":"thing",
		"display_name":"Thing",
		"type":"routed"
	}`)
	if err := CreateDocProcessor(c); err != nil {
		t.Fatalf("CreateDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreateDocProcessorInvalidStatusRejected(t *testing.T) {
	_, mock := installPolicyDB(t)

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-processors", `{
		"name_as_id":"thing",
		"display_name":"Thing",
		"type":"configurable",
		"status":"paused"
	}`)
	if err := CreateDocProcessor(c); err != nil {
		t.Fatalf("CreateDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateDocProcessorSuccess(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	// Sorted editable fields for this payload: display_name, type.
	const updateQuery = "UPDATE kb.doc_processors SET modify_time = NOW(), display_name = $1, type = $2 WHERE name_as_id = $3"
	mock.ExpectExec(regexp.QuoteMeta(updateQuery)).
		WithArgs("Blocking Processor v2", "configurable", "blocking").
		WillReturnResult(sqlmock.NewResult(0, 1))
	expectFetchDocProcessor(mock, "blocking", dpRow("blocking", "Blocking Processor v2", nil, "configurable", false, "active", nil))

	c, rec := newDocProcessorNameContext(t, http.MethodPut, "blocking", `{
		"display_name":"Blocking Processor v2",
		"type":"configurable"
	}`)
	if err := UpdateDocProcessor(c); err != nil {
		t.Fatalf("UpdateDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload docProcessorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if payload.Record.DisplayName != "Blocking Processor v2" || payload.Record.Type != "configurable" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateDocProcessorNotFound(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	const updateQuery = "UPDATE kb.doc_processors SET modify_time = NOW(), status = $1 WHERE name_as_id = $2"
	mock.ExpectExec(regexp.QuoteMeta(updateQuery)).
		WithArgs("disabled", "ghost").
		WillReturnResult(sqlmock.NewResult(0, 0))

	c, rec := newDocProcessorNameContext(t, http.MethodPut, "ghost", `{
		"status":"disabled"
	}`)
	if err := UpdateDocProcessor(c); err != nil {
		t.Fatalf("UpdateDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateDocProcessorNameImmutable(t *testing.T) {
	_, mock := installPolicyDB(t)

	c, rec := newDocProcessorNameContext(t, http.MethodPut, "blocking", `{
		"name_as_id":"other"
	}`)
	if err := UpdateDocProcessor(c); err != nil {
		t.Fatalf("UpdateDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "immutable") {
		t.Fatalf("expected immutable-name message, got: %s", rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateDocProcessorInvalidTypeRejected(t *testing.T) {
	_, mock := installPolicyDB(t)

	c, rec := newDocProcessorNameContext(t, http.MethodPut, "blocking", `{
		"type":"routed"
	}`)
	if err := UpdateDocProcessor(c); err != nil {
		t.Fatalf("UpdateDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateDocProcessorNoEditableFields(t *testing.T) {
	_, mock := installPolicyDB(t)

	c, rec := newDocProcessorNameContext(t, http.MethodPut, "blocking", `{}`)
	if err := UpdateDocProcessor(c); err != nil {
		t.Fatalf("UpdateDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no editable fields") {
		t.Fatalf("expected no-editable-fields message, got: %s", rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestDeleteDocProcessorSuccess(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectExec(regexp.QuoteMeta(dpDeleteQuery)).
		WithArgs("extract_metrics").
		WillReturnResult(sqlmock.NewResult(0, 1))

	c, rec := newDocProcessorNameContext(t, http.MethodDelete, "extract_metrics", "")
	if err := DeleteDocProcessor(c); err != nil {
		t.Fatalf("DeleteDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload docProcessorDeleteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || payload.Deleted != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestDeleteDocProcessorNotFound(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectExec(regexp.QuoteMeta(dpDeleteQuery)).
		WithArgs("ghost").
		WillReturnResult(sqlmock.NewResult(0, 0))

	c, rec := newDocProcessorNameContext(t, http.MethodDelete, "ghost", "")
	if err := DeleteDocProcessor(c); err != nil {
		t.Fatalf("DeleteDocProcessor returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
