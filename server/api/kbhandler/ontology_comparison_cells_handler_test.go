package kbhandler

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestCreateOntologyComparisonCellPersistsDirectionalEvidence(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.ontology_comparison_cells")).WithArgs(int64(7), "obj", "metric", "enterprise", int64(1), `[]`, int64(0), "authority", int64(2), `[]`, int64(0), "stronger", "subject_to_authority", "tighter").WillReturnResult(sqlmock.NewResult(1, 1))
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/comparison-runs/7/cells", `{"target_object_id":"obj","metric_key":"metric","subject_family":"enterprise","subject_representative_assertion_id":1,"authority_family":"authority","authority_representative_assertion_id":2,"verdict":"stronger","direction":"subject_to_authority","rationale":"tighter"}`, map[string]string{"run_id": "7"})
	if err := CreateOntologyComparisonCell(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
