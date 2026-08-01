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

func TestListActiveOntologyProfileRulesReturnsReleaseBoundContent(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profile_rules pr")).WithArgs("example:profile", 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "rule_id", "version", "profile_id", "profile_version", "rule_kind", "status", "severity", "rule_config", "applicability", "release_id", "create_time", "create_by", "modify_time", "modify_by"}).
			AddRow(1, "example:rule", 1, "example:profile", 1, "required_assertion_pattern", "included_in_release", "error", []byte(`{}`), []byte(`{}`), 42, now, "author", now, "author"))
	c, rec := newOntologyCandidateContext(t, http.MethodGet, "/api/v1/kb/ontology/profile-rules?profile_id=example:profile&profile_version=1", "", nil)
	if err := ListActiveOntologyProfileRules(c); err != nil {
		t.Fatalf("ListActiveOntologyProfileRules: %v", err)
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

func TestTransitionOntologyProfileRuleUsesGovernedLifecycle(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	old := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = old }()
	now := time.Now()
	columns := []string{"id", "rule_id", "version", "profile_id", "profile_version", "rule_kind", "status", "severity", "rule_config", "applicability", "create_time", "create_by", "modify_time", "modify_by"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profile_rules WHERE rule_id = $1 AND version = $2")).WithArgs("example:rule", 1).WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "example:rule", 1, "p", 1, "required_assertion_pattern", "draft", "error", []byte(`{}`), []byte(`{}`), now, "", now, ""))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_profile_rules SET status = $3")).WithArgs("example:rule", 1, "in_review", "curator").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profile_rules WHERE rule_id = $1 AND version = $2")).WithArgs("example:rule", 1).WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "example:rule", 1, "p", 1, "required_assertion_pattern", "in_review", "error", []byte(`{}`), []byte(`{}`), now, "", now, "curator"))
	c, rec := newOntologyCandidateContext(t, http.MethodPost, "/api/v1/kb/ontology/profile-rules/example:rule/1/status", `{"to":"in_review","by":"curator"}`, map[string]string{"rule_id": "example:rule", "version": "1"})
	if err := TransitionOntologyProfileRuleStatus(c); err != nil {
		t.Fatalf("TransitionOntologyProfileRuleStatus: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
