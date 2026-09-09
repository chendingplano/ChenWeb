package kbhandler

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newMetricGraphContext(t *testing.T, metricID string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/kb/metrics/"+metricID+"/graph", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("metric_id")
	c.SetParamValues(metricID)
	return c, rec
}

// TestGetMetricGraphMalformedID: an unparseable metric_id is a 400 and no query
// is run.
func TestGetMetricGraphMalformedID(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()

	c, rec := newMetricGraphContext(t, "not-a-metric-id")
	if err := GetMetricGraph(c); err != nil {
		t.Fatalf("GetMetricGraph: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected DB call for a malformed id: %v", err)
	}
}

// TestGetMetricGraphUnknownMetric: a well-formed id whose kb.metrics row is
// absent is a 404, and only the metric lookup runs.
func TestGetMetricGraphUnknownMetric(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()

	mock.ExpectQuery(`FROM kb\.metrics`).
		WithArgs(int64(999), "999_mtc_9").
		WillReturnError(sql.ErrNoRows)

	c, rec := newMetricGraphContext(t, "999_mtc_9")
	if err := GetMetricGraph(c); err != nil {
		t.Fatalf("GetMetricGraph: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

// TestGetMetricGraphLegacyIDNormalized: a legacy "<record>_<seq>" id is
// accepted and canonicalised before the lookup ("416_1" -> "416_mtc_1").
func TestGetMetricGraphLegacyIDNormalized(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	mock.MatchExpectationsInOrder(false)

	// metric row found; everything downstream empty.
	mock.ExpectQuery(`FROM kb\.metrics`).
		WithArgs(int64(416), "416_mtc_1").
		WillReturnRows(sqlmock.NewRows([]string{
			"metric_name", "metric_name_en", "keyword_concept_id",
			"metric_definition_term_id", "model_name", "is_explicit_metric",
			"confidence", "location_type", "created_at",
		}).AddRow("易腐垃圾收运频次", "", "", "", "deepseek", true, 0.9, "clause", nil))
	mock.ExpectQuery(`FROM kb\.artifact_objects`).
		WillReturnRows(sqlmock.NewRows([]string{"id", "object_name", "object_name_en", "object_id", "reconcile_status"}))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM kb\.semantic_decision_candidates`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`FROM kb\.semantic_decision_candidates`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "logical_identity_key", "revision", "payload_fingerprint", "candidate_kind",
			"proposed_payload", "method", "resolution_outcome", "resolution_reason",
			"source_artifact_type", "source_artifact_id", "input_record_id", "source_line_spans", "confidence",
			"status", "decision_reason", "dependency_fingerprint", "superseded_by",
			"resulting_assertion_id", "last_seen", "create_time", "create_by", "modify_time", "modify_by",
		}))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM kb\.assertion_evidence`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`FROM kb\.assertion_evidence`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "assertion_id", "input_record_id", "artifact_type", "artifact_id",
			"artifact_object_id", "evidence_quote", "source_line_spans", "extraction_run",
			"model", "prompt_version", "confidence", "evidence_role", "actor_kind", "deleted",
			"deleted_reason", "deleted_time", "create_time", "create_by",
		}))
	mock.ExpectQuery(`FROM kb\.semantic_processing_outcomes`).
		WillReturnRows(sqlmock.NewRows([]string{
			"stage_term_id", "disposition_term_id", "execution_status",
			"outcome_category", "finding_count", "highest_severity_term_id", "create_time",
		}))

	c, rec := newMetricGraphContext(t, "416_1")
	if err := GetMetricGraph(c); err != nil {
		t.Fatalf("GetMetricGraph: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	var got metricGraphResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Metric.MetricID != "416_mtc_1" {
		t.Fatalf("metric_id = %q, want 416_mtc_1", got.Metric.MetricID)
	}
	if got.Metric.MetricName != "易腐垃圾收运频次" {
		t.Fatalf("metric_name = %q", got.Metric.MetricName)
	}
	for _, id := range metricGraphChainNodeIDs {
		node, ok := got.Nodes[id]
		if !ok {
			t.Fatalf("node %q missing from payload", id)
		}
		if node.Rows == nil {
			t.Fatalf("node %q rows is nil, want an array", id)
		}
	}
	// proc__extract always carries the one kb.metrics-derived row.
	if len(got.Nodes["proc__extract"].Rows) != 1 {
		t.Fatalf("proc__extract rows = %d, want 1", len(got.Nodes["proc__extract"].Rows))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
