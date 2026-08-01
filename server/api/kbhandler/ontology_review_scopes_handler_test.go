package kbhandler

import (
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestCreateOntologyReviewScopePersistsFrozenSelection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_review_scopes")).
		WithArgs("scope-1", `[]`, `[]`, `[]`, "2026-08-01", "CN", nil, `[{"profile_id":"p","release_id":42}]`, "explicit", `{}`, `[]`, nil, nil).
		WillReturnRows(sqlmock.NewRows([]string{"review_scope_id", "reviewed_document_ids", "target_object_ids", "target_class_term_ids", "as_of_date", "jurisdiction", "operating_context", "selected_profiles", "selection_mode", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time"}).
			AddRow("scope-1", []byte(`[]`), []byte(`[]`), []byte(`[]`), "2026-08-01", "CN", nil, []byte(`[{"profile_id":"p","release_id":42}]`), "explicit", []byte(`{}`), []byte(`[]`), "", "", now))
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/review-scopes", `{"review_scope_id":"scope-1","as_of_date":"2026-08-01","jurisdiction":"CN","selected_profiles":[{"profile_id":"p","release_id":42}]}`, nil)
	if err := CreateOntologyReviewScope(c); err != nil {
		t.Fatalf("CreateOntologyReviewScope: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteOntologyReviewScopeUsesPinnedRulesAndPersistsFinding(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_review_scopes WHERE review_scope_id = $1")).WithArgs("scope").WillReturnRows(sqlmock.NewRows([]string{"review_scope_id", "reviewed_document_ids", "target_object_ids", "target_class_term_ids", "as_of_date", "jurisdiction", "operating_context", "selected_profiles", "selection_mode", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time"}).AddRow("scope", []byte(`[]`), []byte(`[]`), []byte(`[]`), "2026-08-01", "CN", nil, []byte(`[{"profile_id":"p","profile_version":1,"release_id":42}]`), "explicit", []byte(`{}`), []byte(`["d"]`), "", "", now))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profile_rules pr\nJOIN kb.ontology_profiles p")).WithArgs("p", 1, int64(42)).WillReturnRows(sqlmock.NewRows([]string{"id", "rule_id", "version", "profile_id", "profile_version", "rule_kind", "status", "severity", "rule_config", "applicability", "release_id", "create_time", "create_by", "modify_time", "modify_by"}).AddRow(9, "r", 1, "p", 1, "required_assertion_pattern", "included_in_release", "error", []byte(`{"dimension":"d","predicate_term_id":"x","quantifier":"exists_conforming"}`), []byte(`{}`), 42, now, "", now, ""))
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO kb.doc_review_findings")).WithArgs(int64(2), int64(3), "ontology_profile", "profile", "error", "missing", "r", "no qualifying assertion in a closed dimension", "scope", int64(9), int64(0)).WillReturnResult(sqlmock.NewResult(1, 1))
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/review-scopes/scope/execute", `{"input_record_id":2,"run_id":3,"assertions":[]}`, map[string]string{"scope_id": "scope"})
	if err := ExecuteOntologyReviewScope(c); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
