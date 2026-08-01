package kbhandler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestCreateOntologyProfileCreatesDraftOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_profiles")).
		WithArgs("example:profile", "example", "draft", nil, `{}`, `[]`, "author", "author").
		WillReturnRows(sqlmock.NewRows([]string{"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "create_time", "create_by", "modify_time", "modify_by"}).
			AddRow(1, "example:profile", 1, "example", "draft", "", []byte(`{}`), []byte(`[]`), now, "author", now, "author"))
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/profiles", `{"profile_id":"example:profile","module_id":"example","create_by":"author"}`, nil)
	if err := CreateOntologyProfile(c); err != nil {
		t.Fatalf("CreateOntologyProfile: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListActiveOntologyProfilesReturnsReleaseBoundContent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profiles p")).WithArgs("example").
		WillReturnRows(sqlmock.NewRows([]string{"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "release_id", "version", "create_time", "create_by", "modify_time", "modify_by"}).
			AddRow(1, "example:profile", 1, "example", "included_in_release", "", []byte(`{}`), []byte(`[]`), 42, "1.0.0", now, "author", now, "author"))
	c, rec := newOntologyCandidateContext(t, http.MethodGet, "/api/v1/kb/ontology/profiles?module_id=example", "", nil)
	if err := ListActiveOntologyProfiles(c); err != nil {
		t.Fatalf("ListActiveOntologyProfiles: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Status bool `json:"status"`
		Total  int  `json:"total"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !response.Status || response.Total != 1 {
		t.Fatalf("response: %s", rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTransitionOntologyProfileUsesGovernedLifecycle(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()
	columns := []string{"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "create_time", "create_by", "modify_time", "modify_by"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profiles\nWHERE profile_id = $1 AND version = $2")).WithArgs("example:profile", 1).WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "example:profile", 1, "example", "draft", "", []byte(`{}`), []byte(`[]`), now, "", now, ""))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_profiles SET status = $3")).WithArgs("example:profile", 1, "in_review", "curator").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profiles\nWHERE profile_id = $1 AND version = $2")).WithArgs("example:profile", 1).WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "example:profile", 1, "example", "in_review", "", []byte(`{}`), []byte(`[]`), now, "", now, "curator"))
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/profiles/example:profile/1/status", `{"to":"in_review","by":"curator"}`, map[string]string{"profile_id": "example:profile", "version": "1"})
	if err := TransitionOntologyProfileStatus(c); err != nil {
		t.Fatalf("TransitionOntologyProfileStatus: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
