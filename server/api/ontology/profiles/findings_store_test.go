package profiles

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestFindingStorePersistsOntologyProvenance(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.doc_review_findings")).WithArgs(int64(2), int64(3), "ontology_profile", "profile", "error", "missing", "display metrics", "no assertion", "scope-1", int64(4), int64(5)).WillReturnResult(sqlmock.NewResult(1, 1))
	err = (FindingStore{DB: db}).Persist(context.Background(), OntologyFinding{InputRecordID: 2, RunID: 3, ScopeID: "scope-1", ProfileRuleID: 4, AssertionID: 5, Category: ResultMissing, Severity: "error", Title: "display metrics", Description: "no assertion"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
