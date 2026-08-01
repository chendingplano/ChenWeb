package kbhandler

import (
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
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_comparison_runs")).WithArgs("scope", int64(2), "assertion:3", "comparison-v1").WillReturnRows(sqlmock.NewRows([]string{"id", "comparison_scope_id", "input_record_id", "assertion_watermark", "comparator_version", "create_time"}).AddRow(7, "scope", 2, "assertion:3", "comparison-v1", now))
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/comparison-scopes/scope/runs", `{"input_record_id":2,"assertion_watermark":"assertion:3","comparator_version":"comparison-v1"}`, map[string]string{"scope_id": "scope"})
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
