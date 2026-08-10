package kbhandler

import (
	"encoding/json"
	"fmt"
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

func newPipelineContext(t *testing.T, method, target string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newPipelineIDContext(t *testing.T, method, id string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	target := "/api/v1/kb/pipelines/" + id
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/pipelines/:id")
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

const pipelineSelectColumns = `
    id, name, display_name, description, processors, legacy_equivalent, is_system_default,
    version, status, create_time, modify_time
FROM kb.pipelines`

const pipelineLockQuery = `SELECT id, version FROM kb.pipelines WHERE name = $1 ORDER BY version DESC LIMIT 1 FOR UPDATE`

const pipelineInsertQuery = `
INSERT INTO kb.pipelines (
    name, display_name, description, processors, legacy_equivalent, is_system_default, version, status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, 'active'
)
RETURNING id
`

func TestListPipelinesSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	query := regexp.QuoteMeta("SELECT" + pipelineSelectColumns + "\nORDER BY id")
	rows := sqlmock.NewRows([]string{
		"id", "name", "display_name", "description", "processors", "legacy_equivalent", "is_system_default", "version", "status", "create_time", "modify_time",
	}).AddRow(
		int64(1), "legacy_default", "Legacy Default", nil, `{}`, true, false, 1, "active",
		time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC), time.Date(2026, 4, 1, 10, 0, 0, 0, time.UTC),
	).AddRow(
		int64(2), "narrative_default", "Narrative Default", "Narrative pipeline", `{extract_metrics}`, false, true, 2, "active",
		time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC), time.Date(2026, 4, 2, 10, 0, 0, 0, time.UTC),
	)
	mock.ExpectQuery(query).WillReturnRows(rows)

	c, rec := newPipelineContext(t, http.MethodGet, "/api/v1/kb/pipelines", "")
	if err := ListPipelines(c); err != nil {
		t.Fatalf("ListPipelines returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload listPipelinesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || len(payload.Results) != 2 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Results[0].Name != "legacy_default" || !payload.Results[0].LegacyEquivalent {
		t.Fatalf("unexpected first result: %+v", payload.Results[0])
	}
	if len(payload.Results[1].Processors) != 1 || payload.Results[1].Processors[0] != "extract_metrics" {
		t.Fatalf("unexpected processors: %+v", payload.Results[1].Processors)
	}
	if payload.Results[1].Description == nil || *payload.Results[1].Description != "Narrative pipeline" || !payload.Results[1].IsSystemDefault {
		t.Fatalf("unexpected description/is_system_default: %+v", payload.Results[1])
	}
	if payload.Results[1].Version != 2 || payload.Results[1].PipelineStatus != "active" {
		t.Fatalf("unexpected version/status: %+v", payload.Results[1])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(pipelineLockQuery)).
		WithArgs("narrative_default").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}))
	mock.ExpectQuery(regexp.QuoteMeta(pipelineInsertQuery)).
		WithArgs("narrative_default", "Narrative Default", nil, sqlmock.AnyArg(), false, false, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	mock.ExpectCommit()

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineSelectColumns + "\nWHERE id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "display_name", "description", "processors", "legacy_equivalent", "is_system_default", "version", "status", "create_time", "modify_time",
	}).AddRow(
		int64(2), "narrative_default", "Narrative Default", nil, `{extract_metrics,extract_provisions}`, false, false, 1, "active",
		time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC),
	))

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/pipelines", `{
		"name":"narrative_default",
		"display_name":"Narrative Default",
		"processors":["extract_metrics","extract_provisions"],
		"legacy_equivalent":false
	}`)
	if err := CreatePipeline(c); err != nil {
		t.Fatalf("CreatePipeline returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload pipelineDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if !payload.Status || payload.Record.ID != 2 || payload.Record.Name != "narrative_default" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Record.Version != 1 || payload.Record.PipelineStatus != "active" {
		t.Fatalf("unexpected version/status: %+v", payload.Record)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineRequiresName(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/pipelines", `{"display_name":"No Name"}`)
	if err := CreatePipeline(c); err != nil {
		t.Fatalf("CreatePipeline returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineRejectsFailedClosureValidationBeforeTouchingDB(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	// normalize_assertions requires metrics+provisions; neither producer is
	// selected, so ADR 2026081001 DR8 check 1 must reject this before any
	// DB statement runs (mock has zero expectations set).
	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/pipelines", `{
		"name":"broken",
		"processors":["normalize_assertions"]
	}`)
	if err := CreatePipeline(c); err != nil {
		t.Fatalf("CreatePipeline returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineWritesRulesWithDAGEdges(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(pipelineLockQuery)).
		WithArgs("with-rules").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}))
	mock.ExpectQuery(regexp.QuoteMeta(pipelineInsertQuery)).
		WithArgs("with-rules", nil, nil, sqlmock.AnyArg(), false, false, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO kb.pipeline_rules (
    name, priority, pipeline_id, target_processor, effect, predicate, predicate_checksum,
    required_facets, depends_on_processors, active, approval_status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, true, 'approved'
)`)).
		WithArgs("gate-extract-metrics", 0, int64(9), "extract_metrics", "require", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineSelectColumns + "\nWHERE id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(9)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "display_name", "description", "processors", "legacy_equivalent", "is_system_default", "version", "status", "create_time", "modify_time",
	}).AddRow(
		int64(9), "with-rules", nil, nil, `{extract_metrics}`, false, false, 1, "active",
		time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC), time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC),
	))

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/pipelines", `{
		"name":"with-rules",
		"processors":["extract_metrics"],
		"rules":[{
			"name":"gate-extract-metrics",
			"target_processor":"extract_metrics",
			"effect":"require",
			"depends_on_processors":[]
		}]
	}`)
	if err := CreatePipeline(c); err != nil {
		t.Fatalf("CreatePipeline returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestCreatePipelineSecondVersionSupersedesPrior(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(pipelineLockQuery)).
		WithArgs("narrative_default").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}).AddRow(int64(2), 1))
	mock.ExpectQuery(regexp.QuoteMeta(pipelineInsertQuery)).
		WithArgs("narrative_default", nil, nil, sqlmock.AnyArg(), false, false, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(20)))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.pipelines SET status = 'superseded', modify_time = NOW() WHERE id = $1`)).
		WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineSelectColumns + "\nWHERE id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(20)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "display_name", "description", "processors", "legacy_equivalent", "is_system_default", "version", "status", "create_time", "modify_time",
	}).AddRow(
		int64(20), "narrative_default", nil, nil, `{extract_metrics}`, false, false, 2, "active",
		time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC), time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC),
	))

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/pipelines", `{
		"name":"narrative_default",
		"processors":["extract_metrics"]
	}`)
	if err := CreatePipeline(c); err != nil {
		t.Fatalf("CreatePipeline returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload pipelineDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if payload.Record.Version != 2 {
		t.Fatalf("expected version 2, got: %+v", payload.Record)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestCreatePipelineMidTransactionFailureRollsBackSupersede proves ADR
// 2026081001 DR2's "author a new version atomically, in one transaction":
// if any statement inside the version-authoring transaction fails (here, a
// rule insert), the whole transaction rolls back -- the prior version's
// status is never actually flipped to 'superseded' in the database, even
// though the UPDATE statement was sent before the failing INSERT. This is
// the atomic-authoring equivalent of the retired ActivatePipelinePolicy's
// "failed activation leaves the previous active version untouched"
// guarantee (P5 acceptance criterion 13).
func TestCreatePipelineMidTransactionFailureRollsBackSupersede(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(pipelineLockQuery)).
		WithArgs("narrative_default").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}).AddRow(int64(2), 1))
	mock.ExpectQuery(regexp.QuoteMeta(pipelineInsertQuery)).
		WithArgs("narrative_default", nil, nil, sqlmock.AnyArg(), false, false, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(20)))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.pipelines SET status = 'superseded', modify_time = NOW() WHERE id = $1`)).
		WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`
INSERT INTO kb.pipeline_rules (
    name, priority, pipeline_id, target_processor, effect, predicate, predicate_checksum,
    required_facets, depends_on_processors, active, approval_status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, true, 'approved'
)`)).
		WillReturnError(fmt.Errorf("connection reset"))
	mock.ExpectRollback()

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/pipelines", `{
		"name":"narrative_default",
		"processors":["extract_metrics"],
		"rules":[{"name":"gate-extract-metrics","target_processor":"extract_metrics","effect":"require"}]
	}`)
	if err := CreatePipeline(c); err != nil {
		t.Fatalf("CreatePipeline returned error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rec.Code, rec.Body.String())
	}

	// The UPDATE ...'superseded' statement was sent, but never committed --
	// sqlmock's ExpectRollback (not ExpectCommit) above enforces that.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations (transaction must roll back, not commit): %v", err)
	}
}

func TestCreatePipelineWithDescriptionAndSystemDefault(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(pipelineLockQuery)).
		WithArgs("no-entities-relations").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}))
	mock.ExpectQuery(regexp.QuoteMeta(pipelineInsertQuery)).
		WithArgs("no-entities-relations", nil, "This is the system default policy", sqlmock.AnyArg(), false, true, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))
	mock.ExpectCommit()

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineSelectColumns + "\nWHERE id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "display_name", "description", "processors", "legacy_equivalent", "is_system_default", "version", "status", "create_time", "modify_time",
	}).AddRow(
		int64(5), "no-entities-relations", nil, "This is the system default policy", `{extract_metrics}`, false, true, 1, "active",
		time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC), time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC),
	))

	c, rec := newPipelineContext(t, http.MethodPost, "/api/v1/kb/pipelines", `{
		"name":"no-entities-relations",
		"description":"This is the system default policy",
		"processors":["extract_metrics"],
		"is_system_default":true
	}`)
	if err := CreatePipeline(c); err != nil {
		t.Fatalf("CreatePipeline returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload pipelineDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if payload.Record.Description == nil || *payload.Record.Description != "This is the system default policy" || !payload.Record.IsSystemDefault {
		t.Fatalf("unexpected payload: %+v", payload.Record)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdatePipelineSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta("UPDATE kb.pipelines SET modify_time = NOW(), display_name = $1, legacy_equivalent = $2 WHERE id = $3")
	mock.ExpectExec(updateQuery).
		WithArgs("Updated Display", true, int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineSelectColumns + "\nWHERE id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(2)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "display_name", "description", "processors", "legacy_equivalent", "is_system_default", "version", "status", "create_time", "modify_time",
	}).AddRow(
		int64(2), "narrative_default", "Updated Display", nil, `{extract_metrics}`, true, false, 1, "active",
		time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC), time.Date(2026, 7, 31, 9, 30, 0, 0, time.UTC),
	))

	c, rec := newPipelineIDContext(t, http.MethodPut, "2", `{
		"display_name":"Updated Display",
		"legacy_equivalent":true
	}`)
	if err := UpdatePipeline(c); err != nil {
		t.Fatalf("UpdatePipeline returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdatePipelineRejectsProcessorsEdit(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	// ADR 2026081001 DR1: processors[] is immutable once a version exists;
	// changing it means authoring a new version via CreatePipeline.
	c, rec := newPipelineIDContext(t, http.MethodPut, "2", `{"processors":["extract_metrics"]}`)
	if err := UpdatePipeline(c); err != nil {
		t.Fatalf("UpdatePipeline returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdatePipelineSetsDescriptionAndSystemDefault(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	// UpdatePipeline builds SET clauses from the sorted payload field names,
	// so "description" sorts before "is_system_default".
	updateQuery := regexp.QuoteMeta("UPDATE kb.pipelines SET modify_time = NOW(), description = $1, is_system_default = $2 WHERE id = $3")
	mock.ExpectExec(updateQuery).
		WithArgs("New description", true, int64(5)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	selectQuery := regexp.QuoteMeta("SELECT" + pipelineSelectColumns + "\nWHERE id = $1")
	mock.ExpectQuery(selectQuery).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{
		"id", "name", "display_name", "description", "processors", "legacy_equivalent", "is_system_default", "version", "status", "create_time", "modify_time",
	}).AddRow(
		int64(5), "no-entities-relations", nil, "New description", `{}`, false, true, 1, "active",
		time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC), time.Date(2026, 8, 9, 9, 30, 0, 0, time.UTC),
	))

	c, rec := newPipelineIDContext(t, http.MethodPut, "5", `{
		"description":"New description",
		"is_system_default":true
	}`)
	if err := UpdatePipeline(c); err != nil {
		t.Fatalf("UpdatePipeline returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdatePipelineRejectsEmptyName(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newPipelineIDContext(t, http.MethodPut, "2", `{"name":""}`)
	if err := UpdatePipeline(c); err != nil {
		t.Fatalf("UpdatePipeline returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
