package profiles

import (
	"context"
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
