package kbhandler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	appconfig "github.com/chendingplano/deepdoc/server/cmd/config"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newMetricSearchContext(t *testing.T, rawQuery string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/metrics/search?"+rawQuery, nil)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestSearchMetricsRejectsInvalidExplicitMetricFilter(t *testing.T) {
	c, rec := newMetricSearchContext(t, "q=energy&is_explicit_metric=maybe")
	if err := SearchMetrics(c); err != nil {
		t.Fatalf("SearchMetrics returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestSearchMetricsReturnsRankedHybridResults(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM kb.metrics m WHERE").
		WithArgs("energy", int64(7), true, "performance", "number", "kWh/m2").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(1)))

	mock.ExpectQuery("WITH query_input AS").
		WithArgs("energy", int64(7), true, "performance", "number", "kWh/m2", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "metric_id", "input_record_id", "input_filename", "metric_name", "metric_name_en",
			"metric_subject", "metric_subject_en", "metric_value", "metric_unit", "metric_unit_en",
			"value_class", "value_class_en", "value_data_type", "is_explicit_metric", "table_name_or_section",
			"metric_keywords", "metric_keywords_en", "source_line_spans", "score", "snippet",
		}).AddRow(
			int64(22), "7_1", int64(7), "input_7.pdf", "Energy intensity", "Energy intensity",
			"Building envelope", "Building envelope", "12", "kWh/m2", "kWh/m2",
			"performance", "performance", "number", true, "Table 2",
			`["energy","intensity"]`, `["energy","intensity"]`, `["10:11"]`, 0.9132, "Energy intensity target is 12 kWh/m2",
		))

	c, rec := newMetricSearchContext(t, "q=energy&input_record_id=7&is_explicit_metric=true&value_class=performance&value_data_type=number&metric_unit=kWh%2Fm2")
	if err := SearchMetrics(c); err != nil {
		t.Fatalf("SearchMetrics returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload metricSearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true")
	}
	if payload.Query != "energy" {
		t.Fatalf("query=%q", payload.Query)
	}
	if payload.Total != 1 {
		t.Fatalf("total=%d", payload.Total)
	}
	if payload.PageSize != 20 {
		t.Fatalf("page_size=%d", payload.PageSize)
	}
	if len(payload.Results) != 1 {
		t.Fatalf("results len=%d", len(payload.Results))
	}
	if payload.Results[0].PrimaryLabel != "Energy intensity" {
		t.Fatalf("primary_label=%q", payload.Results[0].PrimaryLabel)
	}
	if payload.Results[0].Snippet == "" || !strings.Contains(payload.Results[0].Snippet, "12 kWh/m2") {
		t.Fatalf("snippet=%q", payload.Results[0].Snippet)
	}
	if payload.AppliedFilters.InputRecordID == nil || *payload.AppliedFilters.InputRecordID != 7 {
		t.Fatalf("applied filters input_record_id=%v", payload.AppliedFilters.InputRecordID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestBuildMetricSearchWhereClauseUsesConfiguredQueryParser(t *testing.T) {
	cfg := metricSearchConfig{
		dictionary:     "english",
		phraseFriendly: false,
	}

	whereSQL, args := buildMetricSearchWhereClause("energy target", metricSearchFilters{}, cfg)

	if !strings.Contains(whereSQL, "plainto_tsquery('english', $1)") {
		t.Fatalf("whereSQL=%q", whereSQL)
	}
	if len(args) != 1 || args[0] != "energy target" {
		t.Fatalf("args=%v", args)
	}
}

func TestBuildMetricSearchWhereClauseAddsCJKSubstringFallback(t *testing.T) {
	cfg := metricSearchConfig{
		dictionary:     "simple",
		phraseFriendly: true,
	}

	whereSQL, args := buildMetricSearchWhereClause("平均瞬时日差", metricSearchFilters{}, cfg)

	if !strings.Contains(whereSQL, "coalesce(m.metric_name, '') ILIKE '%' || $1 || '%'") {
		t.Fatalf("whereSQL=%q", whereSQL)
	}
	if !strings.Contains(whereSQL, "coalesce(m.search_document, '') ILIKE '%' || $1 || '%'") {
		t.Fatalf("whereSQL=%q", whereSQL)
	}
	if len(args) != 1 || args[0] != "平均瞬时日差" {
		t.Fatalf("args=%v", args)
	}
}

func TestQueryMetricSearchResultsEscapesHeadlineOptions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	cfg := metricSearchConfig{
		dictionary:      "simple",
		defaultPageSize: 20,
		maxPageSize:     100,
		previewMaxWords: 18,
		phraseFriendly:  true,
		weights:         appconfig.GetMetricSearchConfig().Weights,
	}

	mock.ExpectQuery(regexp.QuoteMeta("WITH query_input AS")).
		WithArgs("energy", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "metric_id", "input_record_id", "input_filename", "metric_name", "metric_name_en",
			"metric_subject", "metric_subject_en", "metric_value", "metric_unit", "metric_unit_en",
			"value_class", "value_class_en", "value_data_type", "is_explicit_metric", "table_name_or_section",
			"metric_keywords", "metric_keywords_en", "source_line_spans", "score", "snippet",
		}))

	_, err = queryMetricSearchResults(db, "energy", metricSearchFilters{}, 1, 20, cfg)
	if err != nil {
		t.Fatalf("queryMetricSearchResults returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestEscapeSQLLiteralDoublesSingleQuotes(t *testing.T) {
	got := escapeSQLLiteral("FragmentDelimiter=' ... '")
	want := "FragmentDelimiter='' ... ''"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
