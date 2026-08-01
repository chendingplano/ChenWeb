package profiles

import (
	"context"
	"encoding/json"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestActiveProfileRulesExcludeDraft(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profile_rules pr\nJOIN kb.ontology_profiles p")).
		WithArgs("ventilator-display:display_metrics", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "rule_id", "version", "profile_id", "profile_version", "rule_kind", "status", "severity", "rule_config", "applicability", "release_id", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(
			int64(10), "ventilator-display:requires_luminance", 1, "ventilator-display:display_metrics", 1, "required_assertion_pattern", "included_in_release", "error", []byte(`{}`), []byte(`{}`), int64(42), now, "curator", now, "curator",
		))

	got, err := (ProfileRuleStore{DB: db}).ListActiveProfileRules(context.Background(), "ventilator-display:display_metrics", 1)
	if err != nil {
		t.Fatalf("ListActiveProfileRules: %v", err)
	}
	if len(got) != 1 || got[0].RuleID != "ventilator-display:requires_luminance" || got[0].ReleaseID != 42 {
		t.Fatalf("active rules = %#v", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestProfileRuleStoreCreateStartsDraftVersionOne(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.ontology_profile_rules")).
		WithArgs("ventilator-display:requires_luminance", "ventilator-display:display_metrics", 1, "required_assertion_pattern", "draft", "error", `{"dimension":"display_metrics"}`, `{}`, "curator", "curator").
		WillReturnRows(sqlmock.NewRows([]string{"id", "rule_id", "version", "profile_id", "profile_version", "rule_kind", "status", "severity", "rule_config", "applicability", "create_time", "create_by", "modify_time", "modify_by"}).
			AddRow(int64(1), "ventilator-display:requires_luminance", 1, "ventilator-display:display_metrics", 1, "required_assertion_pattern", "draft", "error", []byte(`{"dimension":"display_metrics"}`), []byte(`{}`), now, "curator", now, "curator"))
	got, err := (ProfileRuleStore{DB: db}).CreateProfileRule(context.Background(), ProfileRule{RuleID: "ventilator-display:requires_luminance", ProfileID: "ventilator-display:display_metrics", ProfileVersion: 1, RuleKind: "required_assertion_pattern", Severity: "error", RuleConfig: json.RawMessage(`{"dimension":"display_metrics"}`), Applicability: json.RawMessage(`{}`), CreateBy: "curator", ModifyBy: "curator"})
	if err != nil || got.Status != "draft" || got.Version != 1 {
		t.Fatalf("CreateProfileRule = %#v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestProfileRuleStoreTransitionStatusAllowsDraftToInReview(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now()
	columns := []string{"id", "rule_id", "version", "profile_id", "profile_version", "rule_kind", "status", "severity", "rule_config", "applicability", "create_time", "create_by", "modify_time", "modify_by"}
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profile_rules WHERE rule_id = $1 AND version = $2")).WithArgs("r", 1).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "r", 1, "p", 1, "required_assertion_pattern", "draft", "error", []byte(`{}`), []byte(`{}`), now, "", now, ""))
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_profile_rules SET status = $3")).WithArgs("r", 1, "in_review", "curator").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profile_rules WHERE rule_id = $1 AND version = $2")).WithArgs("r", 1).
		WillReturnRows(sqlmock.NewRows(columns).AddRow(1, "r", 1, "p", 1, "required_assertion_pattern", "in_review", "error", []byte(`{}`), []byte(`{}`), now, "", now, "curator"))
	got, err := (ProfileRuleStore{DB: db}).TransitionStatus(context.Background(), "r", 1, "in_review", "curator")
	if err != nil || got.Status != "in_review" {
		t.Fatalf("TransitionStatus = %#v, %v", got, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
