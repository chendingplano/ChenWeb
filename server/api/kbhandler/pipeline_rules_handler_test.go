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

const pipelineRuleSelectCols = `
    r.id, r.name, r.priority, r.match_input_doc_type, r.match_source_language, r.match_knowledge_store_binding,
    r.pipeline_id, p.name, r.active, r.policy_id, r.create_time, r.modify_time
FROM kb.pipeline_rules r
JOIN kb.pipelines p ON p.id = r.pipeline_id`

func newPipelineRuleContext(t *testing.T, method, target string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newPipelineRuleIDContext(t *testing.T, method, id string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	target := "/api/v1/kb/pipeline-rules/" + id
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/pipeline-rules/:id")
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func TestListPipelineRulesOrderedByPriority(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	query := regexp.QuoteMeta(`
SELECT b.id, COALESCE(b.name, ''), b.priority,
       COALESCE(b.predicate, '{}'::jsonb)::text,
       b.pipeline_id, p.name, b.active, b.policy_id, b.create_time, b.modify_time
FROM kb.pipeline_bindings b
JOIN kb.pipelines p ON p.id = b.pipeline_id
WHERE b.binding_kind = 'conditional'
ORDER BY b.priority DESC, b.id`)
	mock.ExpectQuery(query).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "priority", "predicate", "pipeline_id", "pipeline_name", "active", "policy_id", "create_time", "modify_time",
	}).AddRow(
		int64(2), "pdf-zh", 10, `{"version":1,"expression":{"kind":"all","items":[{"kind":"fact","path":"document.input_doc_type","op":"eq","value":"pdf"},{"kind":"fact","path":"document.source_language","op":"eq","value":"zh"}]}}`, int64(3), "regulated_reference", true, int64(1),
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
	).AddRow(
		int64(3), "rich-canonical", 9, `{"version":1,"expression":{"kind":"any","items":[{"kind":"fact","path":"document.input_doc_type","op":"eq","value":"pdf"}]}}`, int64(3), "regulated_reference", true, int64(1),
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
	))

	c, rec := newPipelineRuleContext(t, http.MethodGet, "/api/v1/kb/pipeline-rules", "")
	if err := ListPipelineRules(c); err != nil {
		t.Fatalf("ListPipelineRules returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload listPipelineRulesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || len(payload.Results) != 1 || payload.Total != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Results[0].PipelineName != "regulated_reference" || payload.Results[0].MatchInputDocType == nil || *payload.Results[0].MatchInputDocType != "pdf" {
		t.Fatalf("unexpected result: %+v", payload.Results[0])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineRuleSuccess(t *testing.T) {
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

	insertQuery := regexp.QuoteMeta(`
INSERT INTO kb.pipeline_bindings (
    name, priority, binding_kind, predicate, predicate_checksum, pipeline_id, active, policy_id
) VALUES (
    $1, $2, 'conditional', $3::jsonb, $4, $5, $6, $7
)
RETURNING id
`)
	mock.ExpectQuery(insertQuery).
		WithArgs("pdf-zh", 10, sqlmock.AnyArg(), sqlmock.AnyArg(), int64(3), true, int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))

	selectQuery := regexp.QuoteMeta(`
SELECT b.id, COALESCE(b.name, ''), b.priority,
       COALESCE(b.predicate, '{}'::jsonb)::text,
       b.pipeline_id, p.name, b.active, b.policy_id, b.create_time, b.modify_time
FROM kb.pipeline_bindings b
JOIN kb.pipelines p ON p.id = b.pipeline_id
WHERE b.id = $1 OR b.legacy_rule_id = $1`)
	mock.ExpectQuery(selectQuery).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "priority", "predicate", "pipeline_id", "pipeline_name", "active", "policy_id", "create_time", "modify_time",
	}).AddRow(
		int64(2), "pdf-zh", 10, `{"version":1,"expression":{"kind":"all","items":[{"kind":"fact","path":"document.input_doc_type","op":"eq","value":"pdf"},{"kind":"fact","path":"document.source_language","op":"eq","value":"zh"}]}}`, int64(3), "regulated_reference", true, int64(1),
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
	))

	c, rec := newPipelineRuleContext(t, http.MethodPost, "/api/v1/kb/pipeline-rules", `{
		"name":"pdf-zh",
		"priority":10,
		"match_input_doc_type":" PDF ",
		"match_source_language":" ZH ",
		"pipeline_id":3
	}`)
	if err := CreatePipelineRule(c); err != nil {
		t.Fatalf("CreatePipelineRule returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload pipelineRuleDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || payload.Record.ID != 2 {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineRuleRequiresNameAndPipelineID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newPipelineRuleContext(t, http.MethodPost, "/api/v1/kb/pipeline-rules", `{"name":"missing-pipeline"}`)
	if err := CreatePipelineRule(c); err != nil {
		t.Fatalf("CreatePipelineRule returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdatePipelineRuleSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	selectCompatQuery := regexp.QuoteMeta(`
SELECT b.id, COALESCE(b.name, ''), b.priority,
       COALESCE(b.predicate, '{}'::jsonb)::text,
       b.pipeline_id, p.name, b.active, b.policy_id, b.create_time, b.modify_time
FROM kb.pipeline_bindings b
JOIN kb.pipelines p ON p.id = b.pipeline_id
WHERE b.id = $1 OR b.legacy_rule_id = $1`)
	rows := sqlmock.NewRows([]string{
		"id", "name", "priority", "predicate", "pipeline_id", "pipeline_name", "active", "policy_id", "create_time", "modify_time",
	}).AddRow(
		int64(2), "pdf-zh", 10, `{"version":1,"expression":{"kind":"all","items":[{"kind":"fact","path":"document.input_doc_type","op":"eq","value":"pdf"},{"kind":"fact","path":"document.source_language","op":"eq","value":"zh"}]}}`, int64(3), "regulated_reference", true, int64(1),
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
	)
	mock.ExpectQuery(selectCompatQuery).WithArgs(int64(2)).WillReturnRows(rows)

	updateQuery := regexp.QuoteMeta("UPDATE kb.pipeline_bindings SET modify_time = NOW(), active = $1, priority = $2 WHERE id = $3 OR legacy_rule_id = $3")
	mock.ExpectExec(updateQuery).
		WithArgs(false, 20, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(selectCompatQuery).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "priority", "predicate", "pipeline_id", "pipeline_name", "active", "policy_id", "create_time", "modify_time",
	}).AddRow(
		int64(2), "pdf-zh", 20, `{"version":1,"expression":{"kind":"all","items":[{"kind":"fact","path":"document.input_doc_type","op":"eq","value":"pdf"},{"kind":"fact","path":"document.source_language","op":"eq","value":"zh"}]}}`, int64(3), "regulated_reference", false, int64(1),
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 10, 30, 0, 0, time.UTC),
	))

	c, rec := newPipelineRuleIDContext(t, http.MethodPut, "2", `{"priority":20,"active":false}`)
	if err := UpdatePipelineRule(c); err != nil {
		t.Fatalf("UpdatePipelineRule returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestDeletePipelineRuleNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	deleteQuery := regexp.QuoteMeta("DELETE FROM kb.pipeline_bindings WHERE id = $1 OR legacy_rule_id = $1")
	mock.ExpectExec(deleteQuery).
		WithArgs(int64(999)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	c, rec := newPipelineRuleIDContext(t, http.MethodDelete, "999", "")
	if err := DeletePipelineRule(c); err != nil {
		t.Fatalf("DeletePipelineRule returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
