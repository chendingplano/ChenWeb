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
    m.confidence, m.is_explicit_metric, m.table_name_or_section, m.reasoning_tags,
    COALESCE(to_char(m.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS created_at
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE m.id = $1
`)
	rows := sqlmock.NewRows([]string{
		"id", "input_record_id", "metric_id", "event_id", "input_filename", "metric_name", "metric_name_en",
		"source_line_spans", "metric_subject", "metric_subject_en", "metric_desc", "metric_desc_en",
		"metric_context", "metric_context_en", "metric_keywords", "metric_keywords_en", "model_name",
		"location_type", "metric_unit", "metric_unit_en", "metric_value", "value_data_type",
		"value_range_type", "value_class", "value_class_en", "formula_or_definition",
		"threshold_or_target", "measurement_frequency", "confidence", "is_explicit_metric",
		"table_name_or_section", "reasoning_tags", "created_at",
	}).AddRow(
			int64(11), int64(7), "7_mtc_1", "evt-11", "input_7.pdf", "Updated Metric", "Updated Metric EN",
		`["5","12:14"]`, "Energy usage", "Energy usage EN", "Metric description", "Metric description EN",
		"Metric context", "Metric context EN", `["energy","intensity"]`, `["energy","intensity"]`, "gpt-4.1",
		"table", "kWh", "kWh", "12.3", "number", "exact", "performance", "performance",
		"Definition", "Threshold", "monthly", 0.82, true, "Table 2", `["named_metric"]`,
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
