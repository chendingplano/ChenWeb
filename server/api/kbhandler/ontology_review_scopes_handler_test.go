package kbhandler

import (
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestCreateDeterministicReviewScopeSelectsAndPersistsFrozenSelection(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()

	// 1. Knowledge-store derivation from kb.inputs.ks_store_id (never client
	//    input).
	mock.ExpectQuery(regexp.QuoteMeta("SELECT ks_store_id FROM kb.inputs WHERE id = ANY($1::bigint[])")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{"ks_store_id"}).AddRow(int64(9)))
	// 2. Pinned released-profile load: the active release pin + the profiles
	//    visible through it. The released profile has a trivial (empty)
	//    applicability predicate, so it applies to every subject.
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_active_releases ar")).
		WillReturnRows(sqlmock.NewRows([]string{"module_id", "release_id", "version", "content_checksum"}).
			AddRow("ventilator-display", int64(42), "0.1.0", "sha256:aaa"))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profiles p\nJOIN kb.ontology_module_releases r ON r.id = p.released_in_release_id")).
		WithArgs(sqlmock.AnyArg()).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "release_id", "release_version", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(
			int64(7), "ventilator-display:display_metrics", 1, "ventilator-display", "included_in_release", "Display metrics", []byte(`{}`), []byte(`[]`), int64(42), "0.1.0", now, "curator", now, "curator",
		))
	mock.ExpectCommit()
	// 3. Per-subject facet observations + object classes + deployment facts.
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.doc_facet_values")).
		WithArgs(int64(101), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{"id", "record_id", "path", "value", "state", "method", "confidence", "evidence", "source_fingerprint", "decision_attempt_id", "invocation_id", "vocabulary_release_id"}))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.semantic_assertions a\nJOIN kb.assertion_evidence e")).
		WithArgs(int64(101)).
		WillReturnRows(sqlmock.NewRows([]string{"a.id", "subject_object_id", "a.object_ref_id", "confidence"}))
	// 4. Scope INSERT with the deterministic P5 provenance columns.
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_review_scopes")).
		WithArgs(
			"scope-det", `[101]`, `[]`, `[]`, "2026-08-01", "US", nil,
			sqlmock.AnyArg(), // selected_profiles JSON derived by the selector
			"deterministic_rule", `{}`, `["display_metrics"]`, nil, "auto",
			int64(9), sqlmock.AnyArg(), "complete", sqlmock.AnyArg(), sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"review_scope_id", "reviewed_document_ids", "target_object_ids", "target_class_term_ids", "as_of_date", "jurisdiction", "operating_context", "selected_profiles", "selection_mode", "precedence_policy", "closed_dimensions", "selected_by", "selection_reason", "create_time", "knowledge_store_id", "selection_attempt_id", "selection_status", "fact_snapshot", "selection_snapshot"}).
			AddRow("scope-det", []byte(`[101]`), []byte(`[]`), []byte(`[]`), "2026-08-01", "US", nil, []byte(`[{"profile_id":"ventilator-display:display_metrics","profile_version":1,"release_id":42}]`), "deterministic_rule", []byte(`{}`), []byte(`["display_metrics"]`), "", "auto", now, int64(9), "attempt-abc", "complete", []byte(`{}`), []byte(`{}`)))

	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/review-scopes", `{"review_scope_id":"scope-det","reviewed_document_ids":[101],"jurisdiction":"US","as_of_date":"2026-08-01","closed_dimensions":["display_metrics"],"selection_mode":"deterministic_rule","selection_reason":"auto"}`, nil)
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

func TestCreateDeterministicReviewScopeRejectsSuppliedSelectedProfiles(t *testing.T) {
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/review-scopes", `{"review_scope_id":"scope-det","reviewed_document_ids":[101],"selected_profiles":[{"profile_id":"p"}],"selection_mode":"deterministic_rule"}`, nil)
	if err := CreateOntologyReviewScope(c); err != nil {
		t.Fatalf("CreateOntologyReviewScope: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 for a deterministic request that supplies selected_profiles", rec.Code)
	}
}
