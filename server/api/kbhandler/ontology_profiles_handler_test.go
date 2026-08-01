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
