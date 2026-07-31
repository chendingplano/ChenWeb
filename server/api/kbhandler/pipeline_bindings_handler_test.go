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

const pipelineBindingSelectCols = `
    b.id, b.ks_store_id, b.pipeline_id, p.name, b.policy_id,
    b.create_time, b.modify_time
FROM kb.pipeline_bindings b
JOIN kb.pipelines p ON p.id = b.pipeline_id`

func newPipelineBindingContext(t *testing.T, method, target string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newPipelineBindingIDContext(t *testing.T, method, id string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	target := "/api/v1/kb/pipeline-bindings/" + id
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/pipeline-bindings/:id")
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func TestListPipelineBindingsFiltersByKSStoreID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	query := regexp.QuoteMeta("SELECT" + pipelineBindingSelectCols + "\nWHERE b.ks_store_id = $1\nORDER BY b.id")
	mock.ExpectQuery(query).
		WithArgs(int64(42)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "ks_store_id", "pipeline_id", "name", "policy_id", "create_time", "modify_time"}).AddRow(
			int64(3), int64(42), int64(2), "narrative_default", int64(1),
			time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
		))

	c, rec := newPipelineBindingContext(t, http.MethodGet, "/api/v1/kb/pipeline-bindings?ks_store_id=42", "")
	if err := ListPipelineBindings(c); err != nil {
		t.Fatalf("ListPipelineBindings returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload listPipelineBindingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || len(payload.Results) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Results[0].PipelineName != "narrative_default" {
		t.Fatalf("unexpected pipeline_name: %+v", payload.Results[0])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineBindingSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	activePolicyQuery := regexp.QuoteMeta("SELECT id FROM kb.pipeline_policies WHERE status = 'active' LIMIT 1")
	mock.ExpectQuery(activePolicyQuery).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))

	insertQuery := regexp.QuoteMeta("INSERT INTO kb.pipeline_bindings (ks_store_id, pipeline_id, policy_id) VALUES ($1, $2, $3) RETURNING id")
	mock.ExpectQuery(insertQuery).
		WithArgs(int64(42), int64(2), int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(3)))

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineBindingSelectCols + "\nWHERE b.id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(3)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "ks_store_id", "pipeline_id", "name", "policy_id", "create_time", "modify_time"}).AddRow(
			int64(3), int64(42), int64(2), "narrative_default", int64(1),
			time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
		))

	c, rec := newPipelineBindingContext(t, http.MethodPost, "/api/v1/kb/pipeline-bindings", `{"ks_store_id":42,"pipeline_id":2}`)
	if err := CreatePipelineBinding(c); err != nil {
		t.Fatalf("CreatePipelineBinding returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload pipelineBindingDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || payload.Record.ID != 3 || payload.Record.PipelineName != "narrative_default" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineBindingRequiresKSStoreIDAndPipelineID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newPipelineBindingContext(t, http.MethodPost, "/api/v1/kb/pipeline-bindings", `{"pipeline_id":2}`)
	if err := CreatePipelineBinding(c); err != nil {
		t.Fatalf("CreatePipelineBinding returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdatePipelineBindingSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta("UPDATE kb.pipeline_bindings SET pipeline_id = $1, modify_time = NOW() WHERE id = $2")
	mock.ExpectExec(updateQuery).
		WithArgs(int64(5), int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineBindingSelectCols + "\nWHERE b.id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(3)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "ks_store_id", "pipeline_id", "name", "policy_id", "create_time", "modify_time"}).AddRow(
			int64(3), int64(42), int64(5), "request_override", int64(1),
			time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 10, 30, 0, 0, time.UTC),
		))

	c, rec := newPipelineBindingIDContext(t, http.MethodPut, "3", `{"pipeline_id":5}`)
	if err := UpdatePipelineBinding(c); err != nil {
		t.Fatalf("UpdatePipelineBinding returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestDeletePipelineBindingNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	deleteQuery := regexp.QuoteMeta("DELETE FROM kb.pipeline_bindings WHERE id = $1")
	mock.ExpectExec(deleteQuery).
		WithArgs(int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	c, rec := newPipelineBindingIDContext(t, http.MethodDelete, "999", "")
	if err := DeletePipelineBinding(c); err != nil {
		t.Fatalf("DeletePipelineBinding returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
