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

func newPipelinePolicyContext(t *testing.T, method, target string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newPipelinePolicyIDContext(t *testing.T, method, id string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	target := "/api/v1/kb/pipeline-policies/" + id + "/activate"
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/pipeline-policies/:id/activate")
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func TestListPipelinePoliciesOrderedByVersionDesc(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	query := regexp.QuoteMeta("SELECT" + pipelinePolicySelectColumns + "\nORDER BY version DESC")
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{
		"id", "version", "status", "source_ref", "checksum", "activated_at", "activated_by", "create_time", "modify_time",
	}).AddRow(
		int64(1), 1, "active", "bootstrap", nil,
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), "system",
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
	))

	c, rec := newPipelinePolicyContext(t, http.MethodGet, "/api/v1/kb/pipeline-policies", "")
	if err := ListPipelinePolicies(c); err != nil {
		t.Fatalf("ListPipelinePolicies returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload listPipelinePoliciesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || len(payload.Results) != 1 || payload.Results[0].Status != "active" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelinePolicyInsertsAsDraft(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	insertQuery := regexp.QuoteMeta(`
INSERT INTO kb.pipeline_policies (version, status, source_ref, checksum)
VALUES ((SELECT COALESCE(MAX(version), 0) + 1 FROM kb.pipeline_policies), 'draft', $1, $2)
RETURNING id
`)
	mock.ExpectQuery(insertQuery).
		WithArgs(nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))

	selectQuery := regexp.QuoteMeta("SELECT" + pipelinePolicySelectColumns + "\nWHERE id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "version", "status", "source_ref", "checksum", "activated_at", "activated_by", "create_time", "modify_time",
	}).AddRow(
		int64(2), 2, "draft", nil, nil, nil, nil,
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
	))

	c, rec := newPipelinePolicyContext(t, http.MethodPost, "/api/v1/kb/pipeline-policies", `{}`)
	if err := CreatePipelinePolicy(c); err != nil {
		t.Fatalf("CreatePipelinePolicy returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload pipelinePolicyDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || payload.Record.ID != 2 || payload.Record.Status != "draft" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestActivatePipelinePolicyArchivesPreviousAndActivatesTarget(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.pipeline_policies SET status = 'archived', modify_time = NOW() WHERE status = 'active'")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.pipeline_policies SET status = 'active', activated_at = NOW(), activated_by = $1, modify_time = NOW() WHERE id = $2")).
		WithArgs("qa-lead", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	selectQuery := regexp.QuoteMeta("SELECT" + pipelinePolicySelectColumns + "\nWHERE id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "version", "status", "source_ref", "checksum", "activated_at", "activated_by", "create_time", "modify_time",
	}).AddRow(
		int64(2), 2, "active", nil, nil,
		time.Date(2026, 7, 31, 11, 0, 0, 0, time.UTC), "qa-lead",
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 11, 0, 0, 0, time.UTC),
	))

	c, rec := newPipelinePolicyIDContext(t, http.MethodPost, "2", `{"activated_by":"qa-lead"}`)
	if err := ActivatePipelinePolicy(c); err != nil {
		t.Fatalf("ActivatePipelinePolicy returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload pipelinePolicyDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || payload.Record.Status != "active" || payload.Record.ActivatedBy == nil || *payload.Record.ActivatedBy != "qa-lead" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestActivatePipelinePolicyNotFoundRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.pipeline_policies SET status = 'archived', modify_time = NOW() WHERE status = 'active'")).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.pipeline_policies SET status = 'active', activated_at = NOW(), activated_by = $1, modify_time = NOW() WHERE id = $2")).
		WithArgs("unknown", int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()

	c, rec := newPipelinePolicyIDContext(t, http.MethodPost, "999", "")
	if err := ActivatePipelinePolicy(c); err != nil {
		t.Fatalf("ActivatePipelinePolicy returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
