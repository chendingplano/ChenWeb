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

var dagFixedTime = time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

func newDAGNameContext(t *testing.T, method, name, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	target := "/api/v1/kb/doc-process-dags/" + name
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/doc-process-dags/:name")
	c.SetParamNames("name")
	c.SetParamValues(name)
	return c, rec
}

const dagNameFetchQuery = "SELECT" + dagPipelineColumns + "\nWHERE name = $1 ORDER BY version DESC LIMIT 1"

const dagPipelineLockQuery = `SELECT id, version FROM kb.pipelines WHERE name = $1 ORDER BY version DESC LIMIT 1 FOR UPDATE`

const dagPipelineInsertQuery = `
INSERT INTO kb.pipelines (
    name, display_name, description, processors, legacy_equivalent, is_system_default, version, status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, 'active'
)
RETURNING id
`

const dagSupersedeQuery = `UPDATE kb.pipelines SET status = 'superseded', modify_time = NOW() WHERE id = $1`

const dagClearDefaultQuery = `UPDATE kb.pipelines SET is_system_default = false, modify_time = NOW() WHERE id = $1`

const dagDefaultLockQuery = `SELECT id FROM kb.pipelines WHERE is_system_default FOR UPDATE`

const dagDuplicateCheckQuery = `SELECT EXISTS(SELECT 1 FROM kb.pipelines WHERE name = $1)`

const dagAnyDefaultQuery = `SELECT EXISTS(SELECT 1 FROM kb.pipelines WHERE is_system_default)`

const dagDeleteExistenceQuery = `SELECT EXISTS(SELECT 1 FROM kb.pipelines WHERE name = $1), EXISTS(SELECT 1 FROM kb.pipelines WHERE name = $1 AND is_system_default)`

const dagDeleteBindingsQuery = `DELETE FROM kb.pipeline_bindings WHERE pipeline_id IN (SELECT id FROM kb.pipelines WHERE name = $1)`

const dagDeleteRulesQuery = `DELETE FROM kb.pipeline_rules WHERE pipeline_id IN (SELECT id FROM kb.pipelines WHERE name = $1)`

const dagDeletePipelinesQuery = `DELETE FROM kb.pipelines WHERE name = $1`

var dagPipelineCols = []string{
	"id", "name", "display_name", "description", "processors", "legacy_equivalent",
	"is_system_default", "version", "status", "create_time", "modify_time",
}

var dagRuleCols = []string{
	"id", "name", "priority", "predicate", "predicate_checksum", "target_processor",
	"effect", "required_facets", "depends_on_processors", "active", "create_time", "modify_time",
}

var dagBindingCols = []string{
	"id", "name", "priority", "ks_store_id", "pipeline_id", "binding_kind", "predicate",
	"active", "tenant_id", "user_id", "input_record_id", "create_time", "modify_time",
}

func dagPipelineRow(id int64, name string, display, desc any, processors string, legacy, isDefault bool, version int) []driver.Value {
	return []driver.Value{id, name, display, desc, processors, legacy, isDefault, version, "active", dagFixedTime, dagFixedTime}
}

func expectFetchCurrentDAG(mock sqlmock.Sqlmock, name string, row []driver.Value) {
	mock.ExpectQuery(regexp.QuoteMeta(dagNameFetchQuery)).
		WithArgs(name).
		WillReturnRows(sqlmock.NewRows(dagPipelineCols).AddRow(row...))
}

func TestListDocProcessDAGsSuccess(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	rows := sqlmock.NewRows([]string{
		"id", "name", "display_name", "description", "processors", "legacy_equivalent",
		"is_system_default", "version", "status", "create_time", "modify_time", "rule_count",
	}).
		AddRow(append(dagPipelineRow(1, "mine", "Mine", nil, `{extract_metrics}`, false, true, 2), 1)...).
		AddRow(append(dagPipelineRow(2, "other", nil, nil, `{chunking}`, true, false, 1), 0)...)
	mock.ExpectQuery(regexp.QuoteMeta(dagListQuery)).WithArgs("").WillReturnRows(rows)

	c, rec := newPipelineContext(t, http.MethodGet, "/api/v1/kb/doc-process-dags", "")
	if err := ListDocProcessDAGs(c); err != nil {
		t.Fatalf("ListDocProcessDAGs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload listDAGsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || len(payload.Results) != 2 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Results[0].Name != "mine" || !payload.Results[0].IsSystemDefault || payload.Results[0].RuleCount != 1 || payload.Results[0].Version != 2 {
		t.Fatalf("unexpected first result: %+v", payload.Results[0])
	}
	if payload.Results[1].DisplayName != nil || payload.Results[1].LegacyEquivalent != true || payload.Results[1].RuleCount != 0 {
		t.Fatalf("unexpected second result: %+v", payload.Results[1])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestGetDocProcessDAGSuccess(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	expectFetchCurrentDAG(mock, "mine", dagPipelineRow(2, "mine", "Mine", nil, `{extract_metrics}`, false, true, 2))

	mock.ExpectQuery(regexp.QuoteMeta(dagRulesQuery)).WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows(dagRuleCols).AddRow(
			int64(11), "gate-extract-metrics", 10, `{"kind":"fact","path":"document.input_doc_type","op":"exists"}`,
			"chk-abc", "extract_metrics", "require", `["document"]`, `{chunking}`, true, dagFixedTime, dagFixedTime,
		))
	mock.ExpectQuery(regexp.QuoteMeta(dagBindingsQuery)).WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows(dagBindingCols).AddRow(
			int64(1), "default-bind", 10, int64(3), int64(2), "store_default", `{}`, true, "", "", int64(0), dagFixedTime, dagFixedTime,
		))

	c, rec := newDAGNameContext(t, http.MethodGet, "mine", "")
	if err := GetDocProcessDAG(c); err != nil {
		t.Fatalf("GetDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload dagDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || payload.Record.Name != "mine" || !payload.Record.IsSystemDefault || payload.Record.Version != 2 {
		t.Fatalf("unexpected record: %+v", payload.Record)
	}
	if len(payload.Record.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %+v", payload.Record.Rules)
	}
	rule := payload.Record.Rules[0]
	if rule.TargetProcessor != "extract_metrics" || rule.Effect != "require" || len(rule.DependsOnProcessors) != 1 || rule.DependsOnProcessors[0] != "chunking" {
		t.Fatalf("unexpected rule: %+v", rule)
	}
	if len(rule.Predicate) == 0 || rule.PredicateChecksum != "chk-abc" {
		t.Fatalf("expected predicate + checksum: %+v", rule)
	}
	if len(payload.Record.Bindings) != 1 || payload.Record.Bindings[0].BindingKind != "store_default" {
		t.Fatalf("unexpected bindings: %+v", payload.Record.Bindings)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestGetDocProcessDAGNotFound(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectQuery(regexp.QuoteMeta(dagNameFetchQuery)).WithArgs("missing").
		WillReturnRows(sqlmock.NewRows(dagPipelineCols))

	c, rec := newDAGNameContext(t, http.MethodGet, "missing", "")
	if err := GetDocProcessDAG(c); err != nil {
		t.Fatalf("GetDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestCreateDocProcessDAGAutoMarksFirstDefault proves the "first DAG becomes
// the system default automatically" scenario: is_system_default is absent and
// no kb.pipelines row holds the flag, so the new row is stored as default.
func TestCreateDocProcessDAGAutoMarksFirstDefault(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(dagDuplicateCheckQuery)).WithArgs("first-dag").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta(dagAnyDefaultQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta(dagPipelineLockQuery)).WithArgs("first-dag").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}))
	mock.ExpectQuery(regexp.QuoteMeta(dagPipelineInsertQuery)).
		WithArgs("first-dag", "First", nil, sqlmock.AnyArg(), false, true, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectCommit()

	expectFetchCurrentDAG(mock, "first-dag", dagPipelineRow(7, "first-dag", "First", nil, `{extract_metrics}`, false, true, 1))
	mock.ExpectQuery(regexp.QuoteMeta(dagRulesQuery)).WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows(dagRuleCols))

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-process-dags", `{
		"name":"first-dag",
		"display_name":"First",
		"processors":["extract_metrics"]
	}`)
	if err := CreateDocProcessDAG(c); err != nil {
		t.Fatalf("CreateDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload dagDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || !payload.Record.IsSystemDefault {
		t.Fatalf("expected auto-marked default, got: %+v", payload.Record)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestCreateDocProcessDAGLeavesIncumbentDefaultUntouched: a new non-default
// DAG must not disturb the existing default flag.
func TestCreateDocProcessDAGLeavesIncumbentDefaultUntouched(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(dagDuplicateCheckQuery)).WithArgs("second-dag").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta(dagAnyDefaultQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(dagPipelineLockQuery)).WithArgs("second-dag").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}))
	mock.ExpectQuery(regexp.QuoteMeta(dagPipelineInsertQuery)).
		WithArgs("second-dag", nil, nil, sqlmock.AnyArg(), false, false, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(8)))
	mock.ExpectCommit()

	expectFetchCurrentDAG(mock, "second-dag", dagPipelineRow(8, "second-dag", nil, nil, `{extract_metrics}`, false, false, 1))
	mock.ExpectQuery(regexp.QuoteMeta(dagRulesQuery)).WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows(dagRuleCols))

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-process-dags", `{
		"name":"second-dag",
		"processors":["extract_metrics"]
	}`)
	if err := CreateDocProcessDAG(c); err != nil {
		t.Fatalf("CreateDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations (no clear-default statement may run): %v", err)
	}
}

func TestCreateDocProcessDAGDuplicateNameRejected(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(dagDuplicateCheckQuery)).WithArgs("mine").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-process-dags", `{
		"name":"mine",
		"processors":["extract_metrics"]
	}`)
	if err := CreateDocProcessDAG(c); err != nil {
		t.Fatalf("CreateDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "already exists") {
		t.Fatalf("error should name the duplicate: %s", rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations (must roll back, not commit): %v", err)
	}
}

// TestCreateDocProcessDAGRejectsEmptyProcessorsBeforeDB: at least one doc
// processor is required, and the rejection happens before any DB statement
// (mock has zero expectations set).
func TestCreateDocProcessDAGRejectsEmptyProcessorsBeforeDB(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-process-dags", `{
		"name":"empty",
		"processors":[]
	}`)
	if err := CreateDocProcessDAG(c); err != nil {
		t.Fatalf("CreateDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "at least one doc processor") {
		t.Fatalf("error should state at least one processor is required: %s", rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestCreateDocProcessDAGRejectsCycleBeforeDB: a cyclic depends_on_processors
// edge set fails DR8 validation before anything is written.
func TestCreateDocProcessDAGRejectsCycleBeforeDB(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-process-dags", `{
		"name":"cyclic",
		"processors":["extract_metrics","extract_provisions"],
		"rules":[
			{"name":"gate-a","target_processor":"extract_metrics","effect":"require","depends_on_processors":["extract_provisions"]},
			{"name":"gate-b","target_processor":"extract_provisions","effect":"require","depends_on_processors":["extract_metrics"]}
		]
	}`)
	if err := CreateDocProcessDAG(c); err != nil {
		t.Fatalf("CreateDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "cycle") {
		t.Fatalf("error should name the cycle: %s", rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations (no partial rows may be written): %v", err)
	}
}

// TestCreateDocProcessDAGClearsIncumbentDefault: creating a DAG that requests
// default clears the incumbent in the same transaction.
func TestCreateDocProcessDAGClearsIncumbentDefault(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(dagDuplicateCheckQuery)).WithArgs("new-default").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta(dagDefaultLockQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	mock.ExpectExec(regexp.QuoteMeta(dagClearDefaultQuery)).WithArgs(int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(dagPipelineLockQuery)).WithArgs("new-default").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}))
	mock.ExpectQuery(regexp.QuoteMeta(dagPipelineInsertQuery)).
		WithArgs("new-default", nil, nil, sqlmock.AnyArg(), false, true, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	mock.ExpectCommit()

	expectFetchCurrentDAG(mock, "new-default", dagPipelineRow(9, "new-default", nil, nil, `{extract_metrics}`, false, true, 1))
	mock.ExpectQuery(regexp.QuoteMeta(dagRulesQuery)).WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows(dagRuleCols))

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-process-dags", `{
		"name":"new-default",
		"processors":["extract_metrics"],
		"is_system_default":true
	}`)
	if err := CreateDocProcessDAG(c); err != nil {
		t.Fatalf("CreateDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreateDocProcessDAGWritesRules(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(dagDuplicateCheckQuery)).WithArgs("with-rules").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectQuery(regexp.QuoteMeta(dagAnyDefaultQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(regexp.QuoteMeta(dagPipelineLockQuery)).WithArgs("with-rules").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}))
	mock.ExpectQuery(regexp.QuoteMeta(dagPipelineInsertQuery)).
		WithArgs("with-rules", nil, nil, sqlmock.AnyArg(), false, false, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(10)))
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO kb.pipeline_rules (
    name, priority, pipeline_id, target_processor, effect, predicate, predicate_checksum,
    required_facets, depends_on_processors, active, approval_status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, true, 'approved'
)`)).
		WithArgs("gate-extract-metrics", 0, int64(10), "extract_metrics", "require", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	expectFetchCurrentDAG(mock, "with-rules", dagPipelineRow(10, "with-rules", nil, nil, `{extract_metrics}`, false, false, 1))
	mock.ExpectQuery(regexp.QuoteMeta(dagRulesQuery)).WithArgs(int64(10)).
		WillReturnRows(sqlmock.NewRows(dagRuleCols))

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/doc-process-dags", `{
		"name":"with-rules",
		"processors":["extract_metrics"],
		"rules":[{"name":"gate-extract-metrics","target_processor":"extract_metrics","effect":"require","depends_on_processors":[]}]
	}`)
	if err := CreateDocProcessDAG(c); err != nil {
		t.Fatalf("CreateDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestUpdateDocProcessDAGProcessorChangeAuthorsNewVersion: changing processors
// authors version 2, supersedes version 1, and keeps the default flag on the
// new version (spec "a new version of the default DAG stays the default").
func TestUpdateDocProcessDAGProcessorChangeAuthorsNewVersion(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	// Pre-read of the current version.
	expectFetchCurrentDAG(mock, "mine", dagPipelineRow(2, "mine", "Mine", nil, `{extract_metrics}`, false, true, 1))

	mock.ExpectBegin()
	// newIsDefault=true (current is default, no flag change): clear incumbent.
	mock.ExpectQuery(regexp.QuoteMeta(dagDefaultLockQuery)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	mock.ExpectExec(regexp.QuoteMeta(dagClearDefaultQuery)).WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// Author version 2: lock prior, insert, supersede.
	mock.ExpectQuery(regexp.QuoteMeta(dagPipelineLockQuery)).WithArgs("mine").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}).AddRow(int64(2), 1))
	mock.ExpectQuery(regexp.QuoteMeta(dagPipelineInsertQuery)).
		WithArgs("mine", "Mine", nil, sqlmock.AnyArg(), false, true, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(20)))
	mock.ExpectExec(regexp.QuoteMeta(dagSupersedeQuery)).WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	expectFetchCurrentDAG(mock, "mine", dagPipelineRow(20, "mine", "Mine", nil, `{extract_metrics,extract_provisions}`, false, true, 2))
	mock.ExpectQuery(regexp.QuoteMeta(dagRulesQuery)).WithArgs(int64(20)).
		WillReturnRows(sqlmock.NewRows(dagRuleCols))

	c, rec := newDAGNameContext(t, http.MethodPut, "mine", `{
		"processors":["extract_metrics","extract_provisions"]
	}`)
	if err := UpdateDocProcessDAG(c); err != nil {
		t.Fatalf("UpdateDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload dagDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if payload.Record.Version != 2 || !payload.Record.IsSystemDefault {
		t.Fatalf("expected new default version 2, got: %+v", payload.Record)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestUpdateDocProcessDAGCosmeticDoesNotNewVersion: only display_name changes,
// so the current version is updated in place and no new row is inserted.
func TestUpdateDocProcessDAGCosmeticDoesNotNewVersion(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	expectFetchCurrentDAG(mock, "mine", dagPipelineRow(2, "mine", nil, nil, `{extract_metrics}`, false, false, 1))

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.pipelines SET modify_time = NOW(), display_name = $1 WHERE id = $2`)).
		WithArgs("Renamed", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	expectFetchCurrentDAG(mock, "mine", dagPipelineRow(2, "mine", "Renamed", nil, `{extract_metrics}`, false, false, 1))
	mock.ExpectQuery(regexp.QuoteMeta(dagRulesQuery)).WithArgs(int64(2)).
		WillReturnRows(sqlmock.NewRows(dagRuleCols))

	c, rec := newDAGNameContext(t, http.MethodPut, "mine", `{"display_name":"Renamed"}`)
	if err := UpdateDocProcessDAG(c); err != nil {
		t.Fatalf("UpdateDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload dagDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if payload.Record.Version != 1 || payload.Record.DisplayName == nil || *payload.Record.DisplayName != "Renamed" {
		t.Fatalf("expected in-place cosmetic update (still version 1), got: %+v", payload.Record)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestUpdateDocProcessDAGUnsetOnlyDefaultRejected: the sole system default
// cannot be unset; the request fails before any write.
func TestUpdateDocProcessDAGUnsetOnlyDefaultRejected(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	expectFetchCurrentDAG(mock, "mine", dagPipelineRow(2, "mine", "Mine", nil, `{extract_metrics}`, false, true, 1))

	c, rec := newDAGNameContext(t, http.MethodPut, "mine", `{"is_system_default":false}`)
	if err := UpdateDocProcessDAG(c); err != nil {
		t.Fatalf("UpdateDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "default") {
		t.Fatalf("error should mention the default invariant: %s", rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations (no write may run): %v", err)
	}
}

func TestDeleteDocProcessDAGDefaultRejected(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(dagDeleteExistenceQuery)).WithArgs("mine").
		WillReturnRows(sqlmock.NewRows([]string{"exists", "is_default"}).AddRow(true, true))
	mock.ExpectRollback()

	c, rec := newDAGNameContext(t, http.MethodDelete, "mine", "")
	if err := DeleteDocProcessDAG(c); err != nil {
		t.Fatalf("DeleteDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "default") {
		t.Fatalf("error should mention promoting another default: %s", rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestDeleteDocProcessDAGRemovesAllTablesAtomically: bindings, rules, and
// every pipeline version are removed in one transaction (FK RESTRICT order).
func TestDeleteDocProcessDAGRemovesAllTablesAtomically(t *testing.T) {
	db, mock := installPolicyDB(t)
	_ = db

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(dagDeleteExistenceQuery)).WithArgs("mine").
		WillReturnRows(sqlmock.NewRows([]string{"exists", "is_default"}).AddRow(true, false))
	mock.ExpectExec(regexp.QuoteMeta(dagDeleteBindingsQuery)).WithArgs("mine").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(dagDeleteRulesQuery)).WithArgs("mine").
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(regexp.QuoteMeta(dagDeletePipelinesQuery)).WithArgs("mine").
		WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectCommit()

	c, rec := newDAGNameContext(t, http.MethodDelete, "mine", "")
	if err := DeleteDocProcessDAG(c); err != nil {
		t.Fatalf("DeleteDocProcessDAG returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload dagDeleteResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || payload.Deleted != 3 {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListDocProcessProcessorsSuccess(t *testing.T) {
	c, rec := newPipelineContext(t, http.MethodGet, "/api/v1/kb/doc-process-processors", "")
	if err := ListDocProcessProcessors(c); err != nil {
		t.Fatalf("ListDocProcessProcessors returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var raw struct {
		Status  bool             `json:"status"`
		Results []map[string]any `json:"results"`
		Total   int              `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !raw.Status || raw.Total != len(raw.Results) || raw.Total == 0 {
		t.Fatalf("unexpected processor catalog: %+v", raw)
	}
	hasChunking := false
	for _, r := range raw.Results {
		if r["name"] == "chunking" {
			hasChunking = true
		}
	}
	if !hasChunking {
		t.Fatalf("processor catalog should include chunking: %+v", raw.Results)
	}
}
