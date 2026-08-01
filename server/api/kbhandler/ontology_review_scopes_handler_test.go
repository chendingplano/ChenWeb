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
