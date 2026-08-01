package kbhandler

import (
	"database/sql"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestCreateOntologyComparisonRunPinsProvenance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT comparison_scope_id, target_object_ids")).WithArgs("scope").
		WillReturnRows(sqlmock.NewRows([]string{"comparison_scope_id", "target_object_ids", "metric_keys", "as_of_date", "module_releases", "profile_releases", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time"}).
			AddRow("scope", []byte(`["display-1"]`), []byte(`["time_to_alarm"]`), "2026-08-01", []byte(`[]`), []byte(`[]`), []byte(`{}`), []byte(`{}`), "", "", now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(MAX(id), 0) FROM kb.semantic_assertions")).WithArgs("display-1").
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(int64(3)))
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_comparison_runs")).WithArgs("scope", int64(2), "assertion:3", "comparison-v1").WillReturnRows(sqlmock.NewRows([]string{"id", "comparison_scope_id", "input_record_id", "assertion_watermark", "comparator_version", "create_time"}).AddRow(7, "scope", 2, "assertion:3", "comparison-v1", now))
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/comparison-scopes/scope/runs", `{"input_record_id":2,"comparator_version":"comparison-v1"}`, map[string]string{"scope_id": "scope"})
	if err := CreateOntologyComparisonRun(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateOntologyComparisonRunIgnoresClientSuppliedWatermark(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT comparison_scope_id, target_object_ids")).WithArgs("scope").
		WillReturnRows(sqlmock.NewRows([]string{"comparison_scope_id", "target_object_ids", "metric_keys", "as_of_date", "module_releases", "profile_releases", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time"}).
			AddRow("scope", []byte(`["display-1"]`), []byte(`[]`), "2026-08-01", []byte(`[]`), []byte(`[]`), []byte(`{}`), []byte(`{}`), "", "", now))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(MAX(id), 0) FROM kb.semantic_assertions")).WithArgs("display-1").
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(int64(3)))
	// A forged watermark in the request body must be ignored: the INSERT below
	// only matches the server-computed "assertion:3", not "assertion:99999".
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_comparison_runs")).WithArgs("scope", int64(2), "assertion:3", "comparison-v1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "comparison_scope_id", "input_record_id", "assertion_watermark", "comparator_version", "create_time"}).AddRow(7, "scope", 2, "assertion:3", "comparison-v1", now))
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/comparison-scopes/scope/runs", `{"input_record_id":2,"assertion_watermark":"assertion:99999","comparator_version":"comparison-v1"}`, map[string]string{"scope_id": "scope"})
	if err := CreateOntologyComparisonRun(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateOntologyComparisonRunReturnsNotFoundForMissingScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	mock.ExpectQuery(regexp.QuoteMeta("SELECT comparison_scope_id, target_object_ids")).WithArgs("ghost").WillReturnError(sql.ErrNoRows)
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/comparison-scopes/ghost/runs", `{"input_record_id":2,"comparator_version":"comparison-v1"}`, map[string]string{"scope_id": "ghost"})
	if err := CreateOntologyComparisonRun(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
