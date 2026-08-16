package kbhandler

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/assertions"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newRangeTypeErrorsContext(t *testing.T, query url.Values) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/metrics/range-type-errors?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	for k := range query {
		c.QueryParams().Set(k, query.Get(k))
	}
	return c, rec
}

var rangeTypeErrorColumns = []string{
	"id", "input_record_id", "metric_id", "event_id", "input_filename", "metric_name", "metric_name_en",
	"source_line_spans", "metric_subject", "metric_subject_en", "metric_desc", "metric_desc_en",
	"metric_context", "metric_context_en", "metric_keywords", "metric_keywords_en", "model_name",
	"location_type", "metric_unit", "metric_unit_en", "metric_value", "value_data_type",
	"value_range_type", "value_class", "value_class_en", "formula_or_definition",
	"threshold_or_target", "measurement_frequency", "confidence", "is_explicit_metric",
	"document_title", "document_doc_no", "table_name_or_section", "reasoning_tags", "created_at",
	"value_range_type_error",
}

func addRangeTypeErrorRow(rows *sqlmock.Rows, id int64, valueRangeType, errMsg string) *sqlmock.Rows {
	return rows.AddRow(
		id, int64(244), "metric-"+strconv.FormatInt(id, 10), "evt-"+strconv.FormatInt(id, 10), "input_244.pdf",
		"Metric "+strconv.FormatInt(id, 10), "Metric "+strconv.FormatInt(id, 10), `["100"]`, "subject", "subject",
		"desc", "desc", "context", "context", `[]`, `[]`, "model", "paragraph", "mmHg", "mmHg",
		"250", "number", valueRangeType, "requirement", "requirement",
		"", "", "", 0.95, true, "title", "doc-no", "section", `[]`, "2026-08-14T10:00:00+00:00",
		errMsg,
	)
}

// TestListMetricRangeTypeErrorsNoFilters locks in the base query shape: only
// the value_range_type_error IS NOT NULL condition, no optional filters.
func TestListMetricRangeTypeErrorsNoFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
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
    m.table_name_or_section, m.reasoning_tags,
    COALESCE(to_char(m.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS created_at,
    m.value_range_type_error
FROM kb.metrics m
LEFT JOIN kb.inputs i ON i.id = m.input_record_id
WHERE m.value_range_type_error IS NOT NULL
ORDER BY m.created_at DESC
LIMIT 500
`)
	rows := sqlmock.NewRows(rangeTypeErrorColumns)
	rows = addRangeTypeErrorRow(rows, 1, "threshold_min", `unmapped value_range_type: "threshold_min"`)
	mock.ExpectQuery(query).WithArgs().WillReturnRows(rows)

	c, rec := newRangeTypeErrorsContext(t, url.Values{})
	if err := ListMetricRangeTypeErrors(c); err != nil {
		t.Fatalf("ListMetricRangeTypeErrors returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("expected total=1 in body, got %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestListMetricRangeTypeErrorsWithFilters locks in that all four filters
// combine with AND, in the fixed order input_record_id, date_from, date_to,
// value_range_type, and that value_range_type is normalized before use.
func TestListMetricRangeTypeErrorsWithFilters(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	query := regexp.QuoteMeta(`
WHERE m.value_range_type_error IS NOT NULL AND m.input_record_id = $1 AND m.created_at >= $2::timestamptz AND m.created_at <= $3::timestamptz AND lower(regexp_replace(trim(m.value_range_type), '[- ]', '_', 'g')) = $4
ORDER BY m.created_at DESC
LIMIT 500
`)
	rows := sqlmock.NewRows(rangeTypeErrorColumns)
	mock.ExpectQuery(query).
		WithArgs(int64(244), "2026-08-01", "2026-08-14", "threshold_min").
		WillReturnRows(rows)

	c, _ := newRangeTypeErrorsContext(t, url.Values{
		"input_record_id":  {"244"},
		"date_from":        {"2026-08-01"},
		"date_to":          {"2026-08-14"},
		"value_range_type": {"Threshold-Min"},
	})
	if err := ListMetricRangeTypeErrors(c); err != nil {
		t.Fatalf("ListMetricRangeTypeErrors returned error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestListMetricRangeTypeErrorsInvalidRecordID(t *testing.T) {
	c, rec := newRangeTypeErrorsContext(t, url.Values{"input_record_id": {"not-a-number"}})
	if err := ListMetricRangeTypeErrors(c); err != nil {
		t.Fatalf("ListMetricRangeTypeErrors returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

// TestListValueRangeTypeMapEntries locks in the Map Block ordering: invalid
// (status != 'approved') entries first, then by occurrence_count desc.
func TestListValueRangeTypeMapEntries(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	query := regexp.QuoteMeta(`
SELECT
    raw_value, canonical_bucket, status, occurrence_count, first_seen_record_id, last_seen_record_id, note,
    COALESCE(to_char(create_time, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS create_time, create_by,
    COALESCE(to_char(modify_time, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS modify_time, modify_by
FROM kb.metric_value_range_type_map
ORDER BY (status <> 'approved') DESC, occurrence_count DESC, raw_value ASC`)

	rows := sqlmock.NewRows([]string{
		"raw_value", "canonical_bucket", "status", "occurrence_count", "first_seen_record_id", "last_seen_record_id",
		"note", "create_time", "create_by", "modify_time", "modify_by",
	}).
		AddRow("threshold_min", nil, "proposed", int64(3), int64(101), int64(244), nil,
			"2026-08-01T00:00:00+00:00", nil, "2026-08-01T00:00:00+00:00", nil).
		AddRow("min", "lower_bound", "approved", int64(17), int64(50), int64(200), nil,
			"2026-08-01T00:00:00+00:00", nil, "2026-08-01T00:00:00+00:00", nil)
	mock.ExpectQuery(query).WillReturnRows(rows)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/metric-value-range-type-map", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := ListValueRangeTypeMapEntries(c); err != nil {
		t.Fatalf("ListValueRangeTypeMapEntries returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"total":2`) {
		t.Fatalf("expected total=2 in body, got %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func newUpsertValueRangeTypeMapContext(t *testing.T, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/metric-value-range-type-map", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// TestUpsertValueRangeTypeMapEntrySuccess locks in requirement 7 end-to-end:
// applying a bucket saves the entry as approved, cascades the correction to
// matching kb.metrics rows, invalidates the cache, and reports the corrected
// count.
func TestUpsertValueRangeTypeMapEntrySuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	upsertQuery := regexp.QuoteMeta(`
INSERT INTO kb.metric_value_range_type_map (raw_value, canonical_bucket, status, note, modify_time)
VALUES ($1, $2, 'approved', $3, now())
ON CONFLICT (raw_value) DO UPDATE
   SET canonical_bucket = EXCLUDED.canonical_bucket,
       status = 'approved',
       note = COALESCE(EXCLUDED.note, kb.metric_value_range_type_map.note),
       modify_time = now()`)
	mock.ExpectExec(upsertQuery).
		WithArgs("threshold_min", "lower_bound", nil).
		WillReturnResult(sqlmock.NewResult(0, 1))

	cascadeQuery := regexp.QuoteMeta(`
UPDATE kb.metrics
   SET value_range_type_error = NULL
 WHERE value_range_type_error IS NOT NULL
   AND lower(regexp_replace(trim(value_range_type), '[- ]', '_', 'g')) = $1`)
	mock.ExpectExec(cascadeQuery).
		WithArgs("threshold_min").
		WillReturnResult(sqlmock.NewResult(0, 3))

	reloadQuery := regexp.QuoteMeta(`SELECT
    raw_value, canonical_bucket, status, occurrence_count, first_seen_record_id, last_seen_record_id, note,
    COALESCE(to_char(create_time, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS create_time, create_by,
    COALESCE(to_char(modify_time, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS modify_time, modify_by FROM kb.metric_value_range_type_map WHERE raw_value = $1`)
	mock.ExpectQuery(reloadQuery).
		WithArgs("threshold_min").
		WillReturnRows(sqlmock.NewRows([]string{
			"raw_value", "canonical_bucket", "status", "occurrence_count", "first_seen_record_id", "last_seen_record_id",
			"note", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow("threshold_min", "lower_bound", "approved", int64(3), int64(101), int64(244), nil,
			"2026-08-01T00:00:00+00:00", nil, "2026-08-15T00:00:00+00:00", nil))

	c, rec := newUpsertValueRangeTypeMapContext(t, `{"raw_value":"Threshold-Min","canonical_bucket":"lower_bound"}`)
	if err := UpsertValueRangeTypeMapEntry(c); err != nil {
		t.Fatalf("UpsertValueRangeTypeMapEntry returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"corrected_count":3`) {
		t.Fatalf("expected corrected_count=3 in body, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"status":"approved"`) {
		t.Fatalf("expected entry status=approved in body, got %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUpsertValueRangeTypeMapEntryMissingCanonicalBucket(t *testing.T) {
	c, rec := newUpsertValueRangeTypeMapContext(t, `{"raw_value":"threshold_min","canonical_bucket":""}`)
	if err := UpsertValueRangeTypeMapEntry(c); err != nil {
		t.Fatalf("UpsertValueRangeTypeMapEntry returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUpsertValueRangeTypeMapEntryMissingRawValue(t *testing.T) {
	c, rec := newUpsertValueRangeTypeMapContext(t, `{"raw_value":"   ","canonical_bucket":"lower_bound"}`)
	if err := UpsertValueRangeTypeMapEntry(c); err != nil {
		t.Fatalf("UpsertValueRangeTypeMapEntry returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

// sqlNormalizeEquivalent mirrors the cascade/filter queries' SQL predicate
// (lower(regexp_replace(trim(v), '[- ]', '_', 'g'))) in Go, using the same
// RE2-compatible character class Postgres accepts. This lets
// TestSQLNormalizePredicateAgreesWithGoNormalize check the SQL and Go
// normalizations stay in sync (design D2) without a live Postgres connection.
var sqlNormalizeCharClass = regexp.MustCompile(`[- ]`)

func sqlNormalizeEquivalent(raw string) string {
	return strings.ToLower(sqlNormalizeCharClass.ReplaceAllString(strings.TrimSpace(raw), "_"))
}

func TestSQLNormalizePredicateAgreesWithGoNormalize(t *testing.T) {
	cases := []string{
		"", "  ", "min", "Min Threshold", "at-least", "AT LEAST",
		"greater-than_or-equal to", "  Upper  Limit  ", "下限", "a -  b",
	}
	for _, raw := range cases {
		want := assertions.NormalizeValueRangeTypeRaw(raw)
		got := sqlNormalizeEquivalent(raw)
		if got != want {
			t.Fatalf("sqlNormalizeEquivalent(%q) = %q, want %q (assertions.NormalizeValueRangeTypeRaw)", raw, got, want)
		}
	}
}

func newApplyValueRangeTypeMapContext(t *testing.T, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/metric-value-range-type-map/apply", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	return c, rec
}

// valueRangeTypeMapEntryFetchQuery is the single-entry lookup both
// UpsertValueRangeTypeMapEntry (reload) and ApplyValueRangeTypeMapEntry
// (pre-flight) issue via fetchValueRangeTypeMapEntry.
var valueRangeTypeMapEntryFetchQuery = regexp.QuoteMeta(`SELECT
    raw_value, canonical_bucket, status, occurrence_count, first_seen_record_id, last_seen_record_id, note,
    COALESCE(to_char(create_time, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS create_time, create_by,
    COALESCE(to_char(modify_time, 'YYYY-MM-DD"T"HH24:MI:SSOF'), '') AS modify_time, modify_by FROM kb.metric_value_range_type_map WHERE raw_value = $1`)

func valueRangeTypeMapEntryRow(rawValue string, bucket any, status string) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"raw_value", "canonical_bucket", "status", "occurrence_count", "first_seen_record_id", "last_seen_record_id",
		"note", "create_time", "create_by", "modify_time", "modify_by",
	}).AddRow(rawValue, bucket, status, int64(9), int64(101), int64(244), nil,
		"2026-08-01T00:00:00+00:00", nil, "2026-08-15T00:00:00+00:00", nil)
}

// TestApplyValueRangeTypeMapEntrySuccess locks in the Apply action: an
// approved entry rewrites value_range_type to its canonical_bucket on every
// matching kb.metrics row and clears the error, reporting the row count.
func TestApplyValueRangeTypeMapEntrySuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(valueRangeTypeMapEntryFetchQuery).
		WithArgs("threshold_min").
		WillReturnRows(valueRangeTypeMapEntryRow("threshold_min", "lower_bound", "approved"))

	applyQuery := regexp.QuoteMeta(`
UPDATE kb.metrics
   SET value_range_type = $2,
       value_range_type_error = NULL
 WHERE lower(regexp_replace(trim(value_range_type), '[- ]', '_', 'g')) = $1
   AND (value_range_type IS DISTINCT FROM $2 OR value_range_type_error IS NOT NULL)`)
	mock.ExpectExec(applyQuery).
		WithArgs("threshold_min", "lower_bound").
		WillReturnResult(sqlmock.NewResult(0, 7))

	c, rec := newApplyValueRangeTypeMapContext(t, `{"raw_value":"Threshold-Min"}`)
	if err := ApplyValueRangeTypeMapEntry(c); err != nil {
		t.Fatalf("ApplyValueRangeTypeMapEntry returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"applied_count":7`) {
		t.Fatalf("expected applied_count=7 in body, got %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestApplyValueRangeTypeMapEntryNotApproved guards the core rule: a
// 'proposed' entry's bucket is a machine guess, so Apply must refuse to write
// it into kb.metrics.
func TestApplyValueRangeTypeMapEntryNotApproved(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(valueRangeTypeMapEntryFetchQuery).
		WithArgs("threshold_min").
		WillReturnRows(valueRangeTypeMapEntryRow("threshold_min", "lower_bound", "proposed"))

	c, rec := newApplyValueRangeTypeMapContext(t, `{"raw_value":"threshold_min"}`)
	if err := ApplyValueRangeTypeMapEntry(c); err != nil {
		t.Fatalf("ApplyValueRangeTypeMapEntry returned error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestApplyValueRangeTypeMapEntryApprovedWithoutBucket covers the approved-but-
// NULL-canonical_bucket row: there is nothing to write, so Apply must refuse
// rather than blank out value_range_type.
func TestApplyValueRangeTypeMapEntryApprovedWithoutBucket(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(valueRangeTypeMapEntryFetchQuery).
		WithArgs("threshold_min").
		WillReturnRows(valueRangeTypeMapEntryRow("threshold_min", nil, "approved"))

	c, rec := newApplyValueRangeTypeMapContext(t, `{"raw_value":"threshold_min"}`)
	if err := ApplyValueRangeTypeMapEntry(c); err != nil {
		t.Fatalf("ApplyValueRangeTypeMapEntry returned error: %v", err)
	}
	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestApplyValueRangeTypeMapEntryUnknownRawValue(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery(valueRangeTypeMapEntryFetchQuery).
		WithArgs("never_seen").
		WillReturnError(sql.ErrNoRows)

	c, rec := newApplyValueRangeTypeMapContext(t, `{"raw_value":"never_seen"}`)
	if err := ApplyValueRangeTypeMapEntry(c); err != nil {
		t.Fatalf("ApplyValueRangeTypeMapEntry returned error: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestApplyValueRangeTypeMapEntryMissingRawValue(t *testing.T) {
	c, rec := newApplyValueRangeTypeMapContext(t, `{"raw_value":"  "}`)
	if err := ApplyValueRangeTypeMapEntry(c); err != nil {
		t.Fatalf("ApplyValueRangeTypeMapEntry returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}
