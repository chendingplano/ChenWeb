package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newRelatedMetricsContext(t *testing.T, metricID, rawQuery string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	target := "/api/v1/kb/metrics/" + metricID + "/related-metrics"
	if rawQuery != "" {
		target += "?" + rawQuery
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("metric_id")
	c.SetParamValues(metricID)
	return c, rec
}

func withMockDB(t *testing.T) (sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	return mock, func() {
		ApiTypes.ProjectDBHandle = old
		db.Close()
	}
}

func decodeRelated(t *testing.T, rec *httptest.ResponseRecorder) relatedMetricsResponse {
	t.Helper()
	var resp relatedMetricsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return resp
}

func TestGetRelatedMetricsMalformedID(t *testing.T) {
	mock, done := withMockDB(t)
	defer done()

	c, rec := newRelatedMetricsContext(t, "nope", "scope=same_class")
	if err := GetRelatedMetrics(c); err != nil {
		t.Fatalf("GetRelatedMetrics: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no DB call expected for a malformed id: %v", err)
	}
}

func TestGetRelatedMetricsBadScope(t *testing.T) {
	mock, done := withMockDB(t)
	defer done()

	c, rec := newRelatedMetricsContext(t, "416_mtc_1", "scope=cousins")
	if err := GetRelatedMetrics(c); err != nil {
		t.Fatalf("GetRelatedMetrics: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("no DB call expected for a bad scope: %v", err)
	}
}

func TestGetRelatedMetricsUnknownMetric(t *testing.T) {
	mock, done := withMockDB(t)
	defer done()

	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM kb\.metrics`).
		WithArgs("777_mtc_2").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

	c, rec := newRelatedMetricsContext(t, "777_mtc_2", "scope=same_class")
	if err := GetRelatedMetrics(c); err != nil {
		t.Fatalf("GetRelatedMetrics: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestGetRelatedMetricsNoResolvedClass(t *testing.T) {
	mock, done := withMockDB(t)
	defer done()

	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM kb\.metrics`).
		WithArgs("416_mtc_1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// resolveClassTermID: no rows -> "" -> valid empty result.
	mock.ExpectQuery(`SELECT DISTINCT a\.instance_of_term_id\s+FROM kb\.semantic_assertions`).
		WithArgs("416_mtc_1").
		WillReturnRows(sqlmock.NewRows([]string{"instance_of_term_id"}))

	c, rec := newRelatedMetricsContext(t, "416_1", "scope=similar_class")
	if err := GetRelatedMetrics(c); err != nil {
		t.Fatalf("GetRelatedMetrics: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	resp := decodeRelated(t, rec)
	if resp.MetricID != "416_mtc_1" {
		t.Fatalf("metric_id = %q, want canonical 416_mtc_1", resp.MetricID)
	}
	if resp.ClassTermID != nil {
		t.Fatalf("class_term_id = %v, want null", *resp.ClassTermID)
	}
	if len(resp.Results) != 0 {
		t.Fatalf("results = %d, want 0", len(resp.Results))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestGetRelatedMetricsSameClass(t *testing.T) {
	mock, done := withMockDB(t)
	defer done()

	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM kb\.metrics`).
		WithArgs("416_mtc_1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT DISTINCT a\.instance_of_term_id\s+FROM kb\.semantic_assertions`).
		WithArgs("416_mtc_1").
		WillReturnRows(sqlmock.NewRows([]string{"instance_of_term_id"}).AddRow("measurement:collection_frequency_x"))
	// sameClass: self-exclusion + dedupe live in SQL ($2 = the selected metric).
	mock.ExpectQuery(`GROUP BY m\.metric_id`).
		WithArgs("measurement:collection_frequency_x", "416_mtc_1", relatedMetricsDefaultSameClass).
		WillReturnRows(sqlmock.NewRows([]string{
			"metric_id", "metric_name", "metric_name_en", "input_record_id", "metric_value", "metric_unit", "staging_filename",
		}).AddRow("812_mtc_3", "收运频次", "collection frequency", int64(812), "2", "times/day", "std_1503937.pdf"))

	c, rec := newRelatedMetricsContext(t, "416_mtc_1", "scope=same_class")
	if err := GetRelatedMetrics(c); err != nil {
		t.Fatalf("GetRelatedMetrics: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	resp := decodeRelated(t, rec)
	if resp.ClassTermID == nil || *resp.ClassTermID != "measurement:collection_frequency_x" {
		t.Fatalf("class_term_id = %v, want measurement:collection_frequency_x", resp.ClassTermID)
	}
	if len(resp.Results) != 1 || resp.Results[0].MetricID != "812_mtc_3" {
		t.Fatalf("results = %+v, want one row for 812_mtc_3", resp.Results)
	}
	if resp.Results[0].ClassTermID != "measurement:collection_frequency_x" {
		t.Fatalf("row class_term_id = %q", resp.Results[0].ClassTermID)
	}
	if resp.Results[0].MatchedClassTermID != "" || resp.Results[0].Score != 0 {
		t.Fatalf("same_class row must not carry matched_class_term_id / score: %+v", resp.Results[0])
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestGetRelatedMetricsSimilarClassRanksByMatchedClassScore(t *testing.T) {
	mock, done := withMockDB(t)
	defer done()

	mock.ExpectQuery(`SELECT EXISTS \(SELECT 1 FROM kb\.metrics`).
		WithArgs("416_mtc_1").
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectQuery(`SELECT DISTINCT a\.instance_of_term_id\s+FROM kb\.semantic_assertions`).
		WithArgs("416_mtc_1").
		WillReturnRows(sqlmock.NewRows([]string{"instance_of_term_id"}).AddRow("measurement:collection_frequency_x"))
	// MatchSimilar: load the self row (has a document, no embedding).
	mock.ExpectQuery(`SELECT search_document, embedding::text\s+FROM kb\.ontology_class_contract_search`).
		WithArgs("measurement:collection_frequency_x").
		WillReturnRows(sqlmock.NewRows([]string{"search_document", "embedding"}).
			AddRow("collection frequency 收运频次 measurement", nil))
	// MatchSimilar: lexical-only fusion query -> two matched classes with scores.
	mock.ExpectQuery(`ontology_class_contract_search`).
		WithArgs("measurement:collection_frequency_x", sqlmock.AnyArg(), similarClassCandidateK).
		WillReturnRows(sqlmock.NewRows([]string{"class_term_id", "score"}).
			AddRow("measurement:pickup_interval_hi", 0.05).
			AddRow("measurement:route_cadence_lo", 0.01))
	// similarClass: expand to instances of the matched classes.
	mock.ExpectQuery(`WHERE rn <= \$4\s+ORDER BY cls`).
		WithArgs(sqlmock.AnyArg(), "measurement:collection_frequency_x", "416_mtc_1", relatedMetricsDefaultSimilar).
		WillReturnRows(sqlmock.NewRows([]string{
			"cls", "metric_id", "metric_name", "metric_name_en", "input_record_id", "metric_value", "metric_unit", "source_filename",
		}).
			AddRow("measurement:route_cadence_lo", "900_mtc_1", "间隔", "cadence", int64(900), "7", "day", "b.pdf").
			AddRow("measurement:pickup_interval_hi", "800_mtc_1", "间隔", "interval", int64(800), "3", "day", "a.pdf"))

	c, rec := newRelatedMetricsContext(t, "416_mtc_1", "scope=similar_class")
	if err := GetRelatedMetrics(c); err != nil {
		t.Fatalf("GetRelatedMetrics: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	resp := decodeRelated(t, rec)
	if len(resp.Results) != 2 {
		t.Fatalf("results = %d, want 2 (%+v)", len(resp.Results), resp.Results)
	}
	// route_cadence_lo (score 0.01) < pickup_interval_hi (score 0.05): the
	// higher-scoring matched class sorts first regardless of DB row order.
	if resp.Results[0].MetricID != "800_mtc_1" || resp.Results[0].MatchedClassTermID != "measurement:pickup_interval_hi" {
		t.Fatalf("row 0 = %+v, want 800_mtc_1 from pickup_interval_hi", resp.Results[0])
	}
	if resp.Results[0].Score <= resp.Results[1].Score {
		t.Fatalf("rows not ordered by score desc: %+v", resp.Results)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
