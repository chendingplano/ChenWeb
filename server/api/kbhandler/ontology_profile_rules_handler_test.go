package kbhandler

import (
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestCreateOntologyProfileRuleCreatesDraftOnly(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_profile_rules")).
		WithArgs("example:rule", "example:profile", 1, "required_assertion_pattern", "draft", "error", `{"dimension":"d","predicate_term_id":"core:p"}`, `{}`, "author", "author").
		WillReturnRows(sqlmock.NewRows([]string{"id", "rule_id", "version", "profile_id", "profile_version", "rule_kind", "status", "severity", "rule_config", "applicability", "create_time", "create_by", "modify_time", "modify_by"}).
			AddRow(1, "example:rule", 1, "example:profile", 1, "required_assertion_pattern", "draft", "error", []byte(`{"dimension":"d","predicate_term_id":"core:p"}`), []byte(`{}`), now, "author", now, "author"))
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/profile-rules", `{"rule_id":"example:rule","profile_id":"example:profile","profile_version":1,"rule_kind":"required_assertion_pattern","rule_config":{"dimension":"d","predicate_term_id":"core:p"},"create_by":"author"}`, nil)
	if err := CreateOntologyProfileRule(c); err != nil {
		t.Fatalf("CreateOntologyProfileRule: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
