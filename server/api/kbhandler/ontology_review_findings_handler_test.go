package kbhandler

import (
	"net/http"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestGetOntologyReviewFindingReturnsProvenance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.doc_review_findings")).WithArgs(int64(5)).WillReturnRows(sqlmock.NewRows([]string{"id", "review_scope_id", "profile_rule_id", "assertion_id"}).AddRow(5, "scope-1", 9, 11))
	c, rec := newOntologyCandidateContext(t, http.MethodGet, "/api/v1/kb/ontology/review-findings/5", "", map[string]string{"id": "5"})
	if err := GetOntologyReviewFinding(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
