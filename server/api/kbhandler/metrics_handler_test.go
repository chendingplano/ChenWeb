package kbhandler

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

type jsonWithFieldMatcher struct {
	field string
	want  any
}

func (m jsonWithFieldMatcher) Match(v driver.Value) bool {
	var raw string
	switch value := v.(type) {
	case string:
		raw = value
	case *string:
		if value == nil {
			return false
		}
		raw = *value
	default:
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return false
	}
	got, ok := payload[m.field]
	if !ok {
		return false
	}
	return reflect.DeepEqual(got, m.want)
}

type stringPtrMatcher string

func (m stringPtrMatcher) Match(v driver.Value) bool {
	switch value := v.(type) {
	case string:
		return value == string(m)
	case *string:
		return value != nil && *value == string(m)
	default:
		return false
	}
}

func strPtrValue(v string) sqlmock.Argument { return stringPtrMatcher(v) }

type int64PtrMatcher int64

func (m int64PtrMatcher) Match(v driver.Value) bool {
	switch value := v.(type) {
	case int64:
		return value == int64(m)
	case *int64:
		return value != nil && *value == int64(m)
	default:
		return false
	}
}

func int64PtrValue(v int64) sqlmock.Argument { return int64PtrMatcher(v) }

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

func newUpdateMetricContext(t *testing.T, id string, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/kb/metrics/"+id, strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/metrics/:id")
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func newListMetricsContext(t *testing.T, inputRecordID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/metrics?input_record_id="+inputRecordID, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.QueryParams().Set("input_record_id", inputRecordID)
	return c, rec
}

func newDeleteInputContext(t *testing.T, id string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/kb/inputs/"+id, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetPath("/api/v1/kb/inputs/:id")
	c.SetParamNames("id")
	c.SetParamValues(id)
	return c, rec
}

func expectInputRelatedColumnCheck(mock sqlmock.Sqlmock, spec inputRelatedDeleteSpec, exists bool) {
	schema, table, _ := strings.Cut(spec.Table, ".")
	mock.ExpectQuery(regexp.QuoteMeta(inputRelatedColumnExistsQuery)).
		WithArgs(schema, table, spec.Column).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
}

func expectDocProcRunLogCleanup(mock sqlmock.Sqlmock, exists bool) {
	for _, spec := range []inputRelatedDeleteSpec{
		{Table: "kb.doc_proc_logs", Column: "run_id"},
		{Table: "kb.doc_process_runs", Column: "id"},
		{Table: "kb.doc_process_runs", Column: "record_id"},
	} {
		expectInputRelatedColumnCheck(mock, spec, exists)
		if !exists {
			return
		}
	}
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.doc_proc_logs WHERE run_id IN (SELECT id FROM kb.doc_process_runs WHERE record_id = $1)`)).
		WithArgs(int64(430)).
		WillReturnResult(sqlmock.NewResult(0, 0))
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

func TestUpdateMetricSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	updateQuery := regexp.QuoteMeta("UPDATE kb.metrics SET confidence = $1, is_explicit_metric = $2, metric_keywords = $3, metric_name = $4 WHERE id = $5")
	mock.ExpectExec(updateQuery).
		WithArgs(0.82, true, `["energy","intensity"]`, "Updated Metric", int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	selectQuery := regexp.QuoteMeta(`
SELECT
    m.id, m.input_record_id, m.metric_id, m.event_id, COALESCE(i.staging_filename, '') AS input_filename,
    m.metric_name, m.metric_name_en, m.source_line_spans, m.metric_subject, m.metric_subject_en,
    m.metric_desc, m.metric_desc_en, m.metric_context, m.metric_context_en,
    m.metric_keywords, m.metric_keywords_en, m.model_name, m.location_type, m.metric_unit, m.metric_unit_en,
    m.metric_value, m.value_data_type, m.value_range_type, m.value_class, m.value_class_en,
    m.formula_or_definition, m.threshold_or_target, m.measurement_frequency,
    m.confidence, m.is_explicit_metric,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'title', i.title, '')), '') AS document_title,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'doc_no', i.doc_no, '')), '') AS document_doc_no,
    ao.object_name,
    m.table_name_or_section, m.reasoning_tags,
    COALESCE(to_char(m.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS created_at
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
LEFT JOIN LATERAL (
    SELECT NULLIF(BTRIM(artifact.object_name), '') AS object_name
    FROM kb.artifact_objects artifact
    WHERE artifact.artifact_type = 'metric'
      AND artifact.artifact_id = m.metric_id
      AND NULLIF(BTRIM(artifact.object_name), '') IS NOT NULL
    ORDER BY
      CASE WHEN COALESCE(artifact.object_role, '') IN ('measured_object', 'self') THEN 0 ELSE 1 END,
      artifact.id
    LIMIT 1
) ao ON TRUE
WHERE m.id = $1
`)
	rows := sqlmock.NewRows([]string{
		"id", "input_record_id", "metric_id", "event_id", "input_filename", "metric_name", "metric_name_en",
		"source_line_spans", "metric_subject", "metric_subject_en", "metric_desc", "metric_desc_en",
		"metric_context", "metric_context_en", "metric_keywords", "metric_keywords_en", "model_name",
		"location_type", "metric_unit", "metric_unit_en", "metric_value", "value_data_type",
		"value_range_type", "value_class", "value_class_en", "formula_or_definition",
		"threshold_or_target", "measurement_frequency", "confidence", "is_explicit_metric",
		"document_title", "document_doc_no", "object_name",
		"table_name_or_section", "reasoning_tags", "created_at",
	}).AddRow(
		int64(11), int64(7), "7_mtc_1", "evt-11", "input_7.pdf", "Updated Metric", "Updated Metric EN",
		`["5","12:14"]`, "Energy usage", "Energy usage EN", "Metric description", "Metric description EN",
		"Metric context", "Metric context EN", `["energy","intensity"]`, `["energy","intensity"]`, "gpt-4.1",
		"table", "kWh", "kWh", "12.3", "number", "exact", "performance", "performance",
		"Definition", "Threshold", "monthly", 0.82, true, "Dynamic BP Spec", "T/JXAS 010—2021", "Adult patient",
		"Table 2", `["named_metric"]`,
		"2026-05-07T13:00:00+00:00",
	)
	mock.ExpectQuery(selectQuery).WithArgs(int64(11)).WillReturnRows(rows)

	c, rec := newUpdateMetricContext(t, "11", `{
		"metric_name":"Updated Metric",
		"metric_keywords":["energy","intensity"],
		"confidence":0.82,
		"is_explicit_metric":true
	}`)
	if err := UpdateMetric(c); err != nil {
		t.Fatalf("UpdateMetric returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload metricDetailResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true")
	}
	if payload.Record.ID != 11 {
		t.Fatalf("expected id=11, got %d", payload.Record.ID)
	}
	if payload.Record.MetricName == nil || *payload.Record.MetricName != "Updated Metric" {
		t.Fatalf("expected updated metric name, got %+v", payload.Record.MetricName)
	}
	if payload.Record.ObjectName == nil || *payload.Record.ObjectName != "Adult patient" {
		t.Fatalf("expected object name, got %+v", payload.Record.ObjectName)
	}
	if payload.Record.DocumentTitle == nil || *payload.Record.DocumentTitle != "Dynamic BP Spec" {
		t.Fatalf("expected document title, got %+v", payload.Record.DocumentTitle)
	}
	if payload.Record.DocumentDocNo == nil || *payload.Record.DocumentDocNo != "T/JXAS 010—2021" {
		t.Fatalf("expected document doc_no, got %+v", payload.Record.DocumentDocNo)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpdateMetricInvalidBoolean(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	c, rec := newUpdateMetricContext(t, "12", `{"is_explicit_metric":"maybe"}`)
	if err := UpdateMetric(c); err != nil {
		t.Fatalf("UpdateMetric returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListMetricsSortsByFirstSourceLineSpan(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	query := regexp.QuoteMeta(`
SELECT
    m.id, m.input_record_id, m.metric_id, m.event_id, COALESCE(i.staging_filename, '') AS input_filename,
    m.metric_name, m.metric_name_en, m.source_line_spans, m.metric_subject, m.metric_subject_en,
    m.metric_desc, m.metric_desc_en, m.metric_context, m.metric_context_en,
    m.metric_keywords, m.metric_keywords_en, m.model_name, m.location_type, m.metric_unit, m.metric_unit_en,
    m.metric_value, m.value_data_type, m.value_range_type, m.value_class, m.value_class_en,
    m.formula_or_definition, m.threshold_or_target, m.measurement_frequency,
    m.confidence, m.is_explicit_metric,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'title', i.title, '')), '') AS document_title,
    NULLIF(BTRIM(COALESCE(i.doc_metadata->>'doc_no', i.doc_no, '')), '') AS document_doc_no,
    ao.object_name,
    m.table_name_or_section, m.reasoning_tags,
    COALESCE(to_char(m.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS created_at
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
LEFT JOIN LATERAL (
    SELECT NULLIF(BTRIM(artifact.object_name), '') AS object_name
    FROM kb.artifact_objects artifact
    WHERE artifact.artifact_type = 'metric'
      AND artifact.artifact_id = m.metric_id
      AND NULLIF(BTRIM(artifact.object_name), '') IS NOT NULL
    ORDER BY
      CASE WHEN COALESCE(artifact.object_role, '') IN ('measured_object', 'self') THEN 0 ELSE 1 END,
      artifact.id
    LIMIT 1
) ao ON TRUE
WHERE m.input_record_id = $1
ORDER BY m.id ASC
`)

	cols := []string{
		"id", "input_record_id", "metric_id", "event_id", "input_filename", "metric_name", "metric_name_en",
		"source_line_spans", "metric_subject", "metric_subject_en", "metric_desc", "metric_desc_en",
		"metric_context", "metric_context_en", "metric_keywords", "metric_keywords_en", "model_name",
		"location_type", "metric_unit", "metric_unit_en", "metric_value", "value_data_type",
		"value_range_type", "value_class", "value_class_en", "formula_or_definition",
		"threshold_or_target", "measurement_frequency", "confidence", "is_explicit_metric",
		"document_title", "document_doc_no", "object_name", "table_name_or_section", "reasoning_tags", "created_at",
	}
	addRow := func(rows *sqlmock.Rows, id int64, name string, spans string) *sqlmock.Rows {
		return rows.AddRow(
			id, int64(244), "metric-"+strconv.FormatInt(id, 10), "evt-"+strconv.FormatInt(id, 10), "input_244.pdf",
			name, name, spans, "subject", "subject", "desc", "desc", "context", "context",
			`[]`, `[]`, "model", "paragraph", "mmHg", "mmHg", "", "number", "exact", "class", "class",
			"", "", "", 0.95, true, "title", "doc-no", "object", "section", `[]`, "2026-07-12T10:00:00+00:00",
		)
	}
	rows := sqlmock.NewRows(cols)
	rows = addRow(rows, 31135, "Metric 54", `["100"]`)
	rows = addRow(rows, 31136, "Metric 55", `["94"]`)
	rows = addRow(rows, 31137, "Metric 56", `["112"]`)
	rows = addRow(rows, 31138, "Metric 57", `["113"]`)
	rows = addRow(rows, 31139, "Metric 58", `["54"]`)
	rows = addRow(rows, 31140, "Metric 59", `["37"]`)
	rows = addRow(rows, 31141, "Metric 60", `["88"]`)
	mock.ExpectQuery(query).WithArgs(int64(244)).WillReturnRows(rows)

	c, rec := newListMetricsContext(t, "244")
	if err := ListMetrics(c); err != nil {
		t.Fatalf("ListMetrics returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload listMetricsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	got := make([]int64, 0, len(payload.Results))
	for _, record := range payload.Results {
		got = append(got, record.ID)
	}
	want := []int64{31140, 31139, 31141, 31136, 31135, 31137, 31138}
	if len(got) != len(want) {
		t.Fatalf("unexpected result count: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order at index %d: got %v want %v", i, got, want)
		}
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestDeleteInputSuccessCascadesRelatedRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	expectResolveInputTablePlural(mock)
	mock.ExpectBegin()
	selectQuery := regexp.QuoteMeta(`SELECT COALESCE(file_name, ''), COALESCE(backup_filename, ''), COALESCE(result_filename, '') FROM kb.inputs WHERE id = $1`)
	mock.ExpectQuery(selectQuery).
		WithArgs(int64(430)).
		WillReturnRows(sqlmock.NewRows([]string{"file_name", "backup_filename", "result_filename"}).AddRow("", "", ""))

	expectDocProcRunLogCleanup(mock, true)
	for _, spec := range inputRelatedDeleteSpecs {
		expectInputRelatedColumnCheck(mock, spec, true)
		stmt := "DELETE FROM " + spec.Table + " WHERE " + spec.Column + " = $1"
		mock.ExpectExec(regexp.QuoteMeta(stmt)).
			WithArgs(int64(430)).
			WillReturnResult(sqlmock.NewResult(0, 0))
	}

	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.inputs WHERE id = $1`)).
		WithArgs(int64(430)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	logInsertQuery := regexp.QuoteMeta(`
INSERT INTO kb.doc_proc_logs (
    call_reason,
    doc_proc_name,
    model_names,
    prompt_name,
    record_id,
    proc_progress,
    entry_type,
    pass,
    llm_call_id,
    activity_name,
    artifact,
    errors,
    extra_info,
    ms_used,
    log_loc,
    prompt_cache_hit_tokens,
    prompt_cache_miss_tokens,
    run_id,
    create_time
) VALUES (
    $1, $2, $3::text[], $4, $5, $6, $7, $8, $9, $10,
    $11::jsonb, $12, $13::jsonb,
    $14, $15, $16, $17, $18, NOW()
)`)
	recordID := int64(430)
	mock.ExpectExec(logInsertQuery).
		WithArgs(
			strPtrValue("delete_input"),
			"delete_input",
			"{}",
			nil,
			int64PtrValue(recordID),
			nil,
			"delete_input",
			nil,
			nil,
			nil,
			nil,
			nil,
			jsonWithFieldMatcher{field: "operation", want: "delete_input"},
			nil,
			deleteInputDocProcLogLoc,
			nil,
			nil,
			nil,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	c, rec := newDeleteInputContext(t, "430")
	if err := DeleteInput(c); err != nil {
		t.Fatalf("DeleteInput returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !payload["status"] {
		t.Fatalf("expected status=true")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestDeleteInputSkipsMissingRelatedCleanupTargets(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	expectDocProcRunLogCleanup(mock, false)
	for _, spec := range inputRelatedDeleteSpecs {
		expectInputRelatedColumnCheck(mock, spec, false)
	}
	mock.ExpectCommit()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin failed: %v", err)
	}
	if err := deleteInputRelatedRows(tx, 430); err != nil {
		t.Fatalf("deleteInputRelatedRows returned error: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestInputRelatedDeleteSpecsCoverArtifactGraphTables(t *testing.T) {
	required := []inputRelatedDeleteSpec{
		{Table: "kb.artifact_connections", Column: "source_record_id"},
		{Table: "kb.artifact_connections", Column: "target_record_id"},
		{Table: "kb.artifact_objects", Column: "source_record_id"},
	}
	for _, want := range required {
		if !slices.Contains(inputRelatedDeleteSpecs, want) {
			t.Fatalf("inputRelatedDeleteSpecs missing %s.%s: DeleteInput would leave orphaned rows in that table", want.Table, want.Column)
		}
	}

	for _, spec := range inputRelatedDeleteSpecs {
		if spec.Table == "kb.object_nodes" {
			t.Fatalf("inputRelatedDeleteSpecs must never delete kb.object_nodes per-record: it is corpus-wide canonical object identity that may still be referenced by other records' kb.artifact_objects rows (ADR 2026070101)")
		}
	}
}

func TestDeleteInputNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	expectResolveInputTablePlural(mock)
	mock.ExpectBegin()
	selectQuery := regexp.QuoteMeta(`SELECT COALESCE(file_name, ''), COALESCE(backup_filename, ''), COALESCE(result_filename, '') FROM kb.inputs WHERE id = $1`)
	mock.ExpectQuery(selectQuery).
		WithArgs(int64(999)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	c, rec := newDeleteInputContext(t, "999")
	if err := DeleteInput(c); err != nil {
		t.Fatalf("DeleteInput returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestPruneMetadataOnlyArtifactWebDirs(t *testing.T) {
	root := t.TempDir()

	leaf := filepath.Join(root, "alpha", "beta")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatalf("mkdir leaf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "alpha", "metadata.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatalf("write alpha metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(leaf, "metadata.txt"), []byte("beta"), 0o644); err != nil {
		t.Fatalf("write beta metadata: %v", err)
	}

	keepDir := filepath.Join(root, "keep")
	if err := os.MkdirAll(keepDir, 0o755); err != nil {
		t.Fatalf("mkdir keep dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(keepDir, "semantic_projections.txt"), []byte("416_proj_1"), 0o644); err != nil {
		t.Fatalf("write keep file: %v", err)
	}

	if err := pruneMetadataOnlyArtifactWebDirs(root); err != nil {
		t.Fatalf("pruneMetadataOnlyArtifactWebDirs returned error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "alpha")); !os.IsNotExist(err) {
		t.Fatalf("expected metadata-only branch removed, stat err=%v", err)
	}
	if _, err := os.Stat(keepDir); err != nil {
		t.Fatalf("expected keep dir to remain: %v", err)
	}
}

func TestRemoveArtifactWebTreeRecordRemovesAllKnownIndexFiles(t *testing.T) {
	root := t.TempDir()
	leaf := filepath.Join(root, "health", "document_management", "version_changes", "dangerous_changes")
	if err := os.MkdirAll(leaf, 0o755); err != nil {
		t.Fatalf("mkdir leaf: %v", err)
	}

	for _, file := range []string{
		"entities.txt",
		"topics.txt",
		"scenes.txt",
		"inventory_items.txt",
		"provisions.txt",
		"summaries.txt",
		"metrics.txt",
		"semantic_projections.txt",
	} {
		body := strings.Join([]string{
			"415_keep_1",
			"430_" + strings.TrimSuffix(file, ".txt") + "_1",
			"431_keep_1",
		}, "\n")
		if err := os.WriteFile(filepath.Join(leaf, file), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
	}
	if err := os.WriteFile(filepath.Join(leaf, "metadata.txt"), []byte("430 should not matter here"), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	stats, err := removeArtifactWebTreeRecord(root, 430)
	if err != nil {
		t.Fatalf("removeArtifactWebTreeRecord returned error: %v", err)
	}
	if stats.FilesScanned != 8 || stats.FilesChanged != 8 || stats.LinesRemoved != 8 {
		t.Fatalf("stats=%+v, want scanned=8 changed=8 removed=8", stats)
	}

	for _, file := range []string{"entities.txt", "topics.txt", "scenes.txt", "inventory_items.txt", "provisions.txt", "summaries.txt", "metrics.txt", "semantic_projections.txt"} {
		body, err := os.ReadFile(filepath.Join(leaf, file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		got := string(body)
		if strings.Contains(got, "430_") {
			t.Fatalf("%s still contains deleted record: %q", file, got)
		}
		if !strings.Contains(got, "415_keep_1") || !strings.Contains(got, "431_keep_1") {
			t.Fatalf("%s lost unrelated rows: %q", file, got)
		}
	}
	metadata, err := os.ReadFile(filepath.Join(leaf, "metadata.txt"))
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	if string(metadata) != "430 should not matter here" {
		t.Fatalf("metadata changed unexpectedly: %q", metadata)
	}
}

func TestArtifactWebCleanupRootsPrefersArtifactWebChild(t *testing.T) {
	parent := t.TempDir()
	child := filepath.Join(parent, "ArtifactWeb")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("mkdir ArtifactWeb child: %v", err)
	}
	t.Setenv("ARTIFACT_WEB_DIR", parent)

	roots := artifactWebCleanupRoots()
	if len(roots) != 1 || roots[0] != child {
		t.Fatalf("roots=%v, want only %q", roots, child)
	}
}

func TestArtifactWebCleanupRootsUsesConfiguredArtifactWeb(t *testing.T) {
	child := filepath.Join(t.TempDir(), "ArtifactWeb")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("mkdir ArtifactWeb: %v", err)
	}
	t.Setenv("ARTIFACT_WEB_DIR", child)

	roots := artifactWebCleanupRoots()
	if len(roots) != 1 || roots[0] != child {
		t.Fatalf("roots=%v, want only %q", roots, child)
	}
}
