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
	"github.com/chendingplano/deepdoc/server/api/ontology/policyaudit"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

const pipelineBindingSelectCols = `
    b.id, COALESCE(b.name, ''), b.priority, COALESCE(b.ks_store_id, 0),
    b.pipeline_id, p.name, b.binding_kind,
    COALESCE(b.predicate, '{}'::jsonb)::text, COALESCE(b.predicate_checksum, ''), b.active,
    COALESCE(b.tenant_id, '-'), COALESCE(b.user_id, ''), COALESCE(b.input_record_id, 0),
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
		WillReturnRows(sqlmock.NewRows([]string{"id", "binding_name", "priority", "ks_store_id", "pipeline_id", "name", "binding_kind", "predicate", "predicate_checksum", "active", "tenant_id", "user_id", "input_record_id", "create_time", "modify_time"}).AddRow(
			int64(3), "store-default", 0, int64(42), int64(2), "narrative_default", "store_default", `{}`, "", true, "-", "", int64(0),
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

func TestListPipelineBindingsReturnsCanonicalScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()
	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	query := regexp.QuoteMeta(`SELECT
    b.id, COALESCE(b.name, ''), b.priority, COALESCE(b.ks_store_id, 0),
    b.pipeline_id, p.name, b.binding_kind,
    COALESCE(b.predicate, '{}'::jsonb)::text, COALESCE(b.predicate_checksum, ''), b.active,
    COALESCE(b.tenant_id, '-'), COALESCE(b.user_id, ''), COALESCE(b.input_record_id, 0),
    b.create_time, b.modify_time
FROM kb.pipeline_bindings b
JOIN kb.pipelines p ON p.id = b.pipeline_id
ORDER BY b.id`)
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{"id", "name", "priority", "ks_store_id", "pipeline_id", "pipeline_name", "binding_kind", "predicate", "predicate_checksum", "active", "tenant_id", "user_id", "input_record_id", "create_time", "modify_time"}).AddRow(
		int64(3), "document policy", 10, int64(42), int64(2), "narrative_default", "conditional", `{}`, "", true, "tenant-a", "user-a", int64(91),
		time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC), time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC),
	))

	c, rec := newPipelineBindingContext(t, http.MethodGet, "/api/v1/kb/pipeline-bindings", "")
	if err := ListPipelineBindings(c); err != nil {
		t.Fatalf("ListPipelineBindings returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`"tenant_id":"tenant-a"`, `"user_id":"user-a"`, `"input_record_id":91`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("response %s missing %s", rec.Body.String(), want)
		}
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
	audit := installPolicyAuditFake(t)

	insertQuery := regexp.QuoteMeta("INSERT INTO kb.pipeline_bindings (name, priority, ks_store_id, pipeline_id, binding_kind, predicate, predicate_checksum, active, tenant_id, user_id, input_record_id) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11) RETURNING id")
	mock.ExpectQuery(insertQuery).
		WithArgs("", 0, int64(42), int64(2), "store_default", nil, "", true, "-", "", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(3)))

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineBindingSelectCols + "\nWHERE b.id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(3)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "binding_name", "priority", "ks_store_id", "pipeline_id", "name", "binding_kind", "predicate", "predicate_checksum", "active", "tenant_id", "user_id", "input_record_id", "create_time", "modify_time"}).AddRow(
			int64(3), "", 0, int64(42), int64(2), "narrative_default", "store_default", `{}`, "", true, "-", "", int64(0),
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
	if len(audit.events) != 1 || audit.events[0].Kind != policyaudit.EventBindingAuthored || audit.events[0].SubjectID != 3 {
		t.Fatalf("audit events=%+v, want one binding_authored event for subject 3", audit.events)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineBindingConditionalValidatesPredicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	insertQuery := regexp.QuoteMeta("INSERT INTO kb.pipeline_bindings (name, priority, ks_store_id, pipeline_id, binding_kind, predicate, predicate_checksum, active, tenant_id, user_id, input_record_id) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11) RETURNING id")
	mock.ExpectQuery(insertQuery).
		WithArgs("pdf policy", 10, nil, int64(2), "conditional", sqlmock.AnyArg(), sqlmock.AnyArg(), true, "-", "", nil).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(4)))

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineBindingSelectCols + "\nWHERE b.id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(4)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "binding_name", "priority", "ks_store_id", "pipeline_id", "name", "binding_kind", "predicate", "predicate_checksum", "active", "tenant_id", "user_id", "input_record_id", "create_time", "modify_time"}).AddRow(
			int64(4), "pdf policy", 10, int64(0), int64(2), "narrative_default", "conditional",
			`{"version":1,"expression":{"kind":"all","items":[{"kind":"fact","path":"document.input_doc_type","op":"eq","value":"pdf"}]}}`,
			"sha256:test", true, "-", "", int64(0),
			time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
		))

	c, rec := newPipelineBindingContext(t, http.MethodPost, "/api/v1/kb/pipeline-bindings", `{
		"name":"pdf policy",
		"priority":10,
		"binding_kind":"conditional",
		"pipeline_id":2,
		"predicate":{"version":1,"expression":{"kind":"all","items":[{"kind":"fact","path":"document.input_doc_type","op":"eq","value":"pdf"}]}}
	}`)
	if err := CreatePipelineBinding(c); err != nil {
		t.Fatalf("CreatePipelineBinding returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineBindingWritesScopeFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	insertQuery := regexp.QuoteMeta("INSERT INTO kb.pipeline_bindings (name, priority, ks_store_id, pipeline_id, binding_kind, predicate, predicate_checksum, active, tenant_id, user_id, input_record_id) VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11) RETURNING id")
	mock.ExpectQuery(insertQuery).
		WithArgs("document policy", 10, int64(42), int64(2), "conditional", sqlmock.AnyArg(), sqlmock.AnyArg(), true, "tenant-a", "user-a", int64(91)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(4)))

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineBindingSelectCols + "\nWHERE b.id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(4)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "binding_name", "priority", "ks_store_id", "pipeline_id", "name", "binding_kind", "predicate", "predicate_checksum", "active", "tenant_id", "user_id", "input_record_id", "create_time", "modify_time"}).AddRow(
			int64(4), "document policy", 10, int64(42), int64(2), "narrative_default", "conditional", `{}`, "", true, "tenant-a", "user-a", int64(91),
			time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC), time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC),
		))

	c, rec := newPipelineBindingContext(t, http.MethodPost, "/api/v1/kb/pipeline-bindings", `{
		"name":"document policy", "priority":10, "ks_store_id":42, "pipeline_id":2,
		"binding_kind":"conditional", "predicate":{"version":1,"expression":{"kind":"all","items":[]}},
		"tenant_id":"tenant-a", "user_id":"user-a", "input_record_id":91
	}`)
	if err := CreatePipelineBinding(c); err != nil {
		t.Fatalf("CreatePipelineBinding returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineBindingRejectsInvalidConditionalPredicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newPipelineBindingContext(t, http.MethodPost, "/api/v1/kb/pipeline-bindings", `{
		"binding_kind":"conditional",
		"pipeline_id":2,
		"predicate":{"version":1,"expression":{"kind":"fact","path":"document.input_doc_type","op":"definitely_not_valid","value":"pdf"}}
	}`)
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
		sqlmock.NewRows([]string{"id", "binding_name", "priority", "ks_store_id", "pipeline_id", "name", "binding_kind", "predicate", "predicate_checksum", "active", "tenant_id", "user_id", "input_record_id", "create_time", "modify_time"}).AddRow(
			int64(3), "", 0, int64(42), int64(5), "request_override", "store_default", `{}`, "", true, "-", "", int64(0),
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

func TestUpdatePipelineBindingUpdatesCanonicalFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta("UPDATE kb.pipeline_bindings SET active = $1, binding_kind = $2, name = $3, pipeline_id = $4, predicate = $5::jsonb, predicate_checksum = $6, priority = $7, modify_time = NOW() WHERE id = $8")
	mock.ExpectExec(updateQuery).
		WithArgs(false, "conditional", "pdf policy v2", int64(5), sqlmock.AnyArg(), sqlmock.AnyArg(), 20, int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineBindingSelectCols + "\nWHERE b.id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(3)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "binding_name", "priority", "ks_store_id", "pipeline_id", "name", "binding_kind", "predicate", "predicate_checksum", "active", "tenant_id", "user_id", "input_record_id", "create_time", "modify_time"}).AddRow(
			int64(3), "pdf policy v2", 20, int64(0), int64(5), "request_override", "conditional",
			`{"version":1,"expression":{"kind":"fact","path":"document.input_doc_type","op":"eq","value":"pdf"}}`,
			"sha256:test", false, "-", "", int64(0),
			time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC), time.Date(2026, 8, 2, 10, 30, 0, 0, time.UTC),
		))

	c, rec := newPipelineBindingIDContext(t, http.MethodPut, "3", `{
		"name":"pdf policy v2",
		"priority":20,
		"binding_kind":"conditional",
		"pipeline_id":5,
		"predicate":{"version":1,"expression":{"kind":"fact","path":"document.input_doc_type","op":"eq","value":"pdf"}},
		"active":false
	}`)
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

func TestUpdatePipelineBindingUpdatesScopeFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta("UPDATE kb.pipeline_bindings SET input_record_id = $1, ks_store_id = $2, tenant_id = $3, user_id = $4, modify_time = NOW() WHERE id = $5")
	mock.ExpectExec(updateQuery).
		WithArgs(int64(91), int64(42), "tenant-a", "user-a", int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineBindingSelectCols + "\nWHERE b.id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(3)).WillReturnRows(
		sqlmock.NewRows([]string{"id", "binding_name", "priority", "ks_store_id", "pipeline_id", "name", "binding_kind", "predicate", "predicate_checksum", "active", "tenant_id", "user_id", "input_record_id", "create_time", "modify_time"}).AddRow(
			int64(3), "document policy", 0, int64(42), int64(2), "request_override", "conditional", `{}`, "", true, "tenant-a", "user-a", int64(91),
			time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC), time.Date(2026, 8, 2, 10, 30, 0, 0, time.UTC),
		))

	c, rec := newPipelineBindingIDContext(t, http.MethodPut, "3", `{"tenant_id":"tenant-a","user_id":"user-a","ks_store_id":42,"input_record_id":91}`)
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
	mock.ExpectExec(deleteQuery).WithArgs(int64(999)).WillReturnResult(sqlmock.NewResult(0, 0))

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
