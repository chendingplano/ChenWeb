package modules

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/api/ontology/profiles"
)

func TestTagContentRowsIncludesProfilesInModuleRelease(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.ontology_profiles SET status = 'included_in_release', released_in_release_id = $1, modify_time = NOW() WHERE id = ANY($2::bigint[])")).
		WithArgs(int64(42), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = tagContentRows(context.Background(), tx, 42, snapshot{
		Profiles: []profiles.Profile{{ID: 9}},
	})
	if err != nil {
		t.Fatalf("tagContentRows: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestBuildSnapshotIncludesApprovedProfiles(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_terms\nWHERE ($1 = '' OR module_id = $1) AND ($2 = '' OR status = $2)")).
		WithArgs("ventilator-display", "approved").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_axioms\nWHERE ($1 = '' OR module_id = $1)")).
		WithArgs("ventilator-display").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_mappings\nWHERE ($1 = '' OR module_id = $1)")).
		WithArgs("ventilator-display").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	now := time.Now()
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profiles\nWHERE module_id = $1 AND status = 'approved'")).
		WithArgs("ventilator-display").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "profile_id", "version", "module_id", "status", "title", "applicability", "closed_dimensions", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(
			int64(9), "ventilator-display:display_metrics", 1, "ventilator-display", "approved", "Display metrics", []byte(`{}`), []byte(`[]`), now, "curator", now, "curator",
		))
	mock.ExpectQuery(regexp.QuoteMeta("FROM kb.ontology_profile_rules pr\nJOIN kb.ontology_profiles p")).
		WithArgs("ventilator-display").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "rule_id", "version", "profile_id", "profile_version", "rule_kind", "status", "severity", "rule_config", "applicability", "create_time", "create_by", "modify_time", "modify_by",
		}).AddRow(
			int64(10), "ventilator-display:requires_luminance", 1, "ventilator-display:display_metrics", 1, "required_assertion_pattern", "approved", "error", []byte(`{}`), []byte(`{}`), now, "curator", now, "curator",
		))

	snap, err := buildSnapshot(context.Background(), db, "ventilator-display", "0.1.0")
	if err != nil {
		t.Fatalf("buildSnapshot: %v", err)
	}
	if len(snap.Profiles) != 1 || snap.Profiles[0].ProfileID != "ventilator-display:display_metrics" {
		t.Fatalf("profiles = %#v", snap.Profiles)
	}
	if len(snap.ProfileRules) != 1 || snap.ProfileRules[0].RuleID != "ventilator-display:requires_luminance" {
		t.Fatalf("profile rules = %#v", snap.ProfileRules)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestValidateDepsRejectsCycles(t *testing.T) {
	mods := []Module{
		{ModuleID: "a", DependsOn: []string{"b"}},
		{ModuleID: "b", DependsOn: []string{"c"}},
		{ModuleID: "c", DependsOn: []string{"a"}},
	}
	if err := validateDeps(mods); err == nil {
		t.Fatal("expected cycle detection")
	}
}

func TestValidateDepsRejectsUnknownDependency(t *testing.T) {
	mods := []Module{
		{ModuleID: "a", DependsOn: []string{"ghost"}},
	}
	if err := validateDeps(mods); err == nil {
		t.Fatal("expected unknown dependency error")
	}
}

func TestValidateDepsAcceptsDAG(t *testing.T) {
	mods := []Module{
		{ModuleID: "core", DependsOn: []string{}},
		{ModuleID: "quantity", DependsOn: []string{"core"}},
		{ModuleID: "measurement", DependsOn: []string{"core", "quantity"}},
	}
	if err := validateDeps(mods); err != nil {
		t.Fatalf("expected valid DAG, got %v", err)
	}
}

func TestChecksumDeterministicAcrossKeyOrder(t *testing.T) {
	a := []byte(`{"module_id":"core","terms":[{"term_id":"core:assertion","definition":"x"}]}`)
	b := []byte(`{"terms":[{"definition":"x","term_id":"core:assertion"}],"module_id":"core"}`)
	ca, err := Checksum(a)
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	cb, err := Checksum(b)
	if err != nil {
		t.Fatalf("Checksum: %v", err)
	}
	if ca != cb {
		t.Fatalf("checksum depends on key order: %s != %s", ca, cb)
	}
}

func TestChecksumDiffersOnContentChange(t *testing.T) {
	a, _ := Checksum([]byte(`{"terms":[{"term_id":"core:assertion"}]}`))
	b, _ := Checksum([]byte(`{"terms":[{"term_id":"core:evidence"}]}`))
	if a == b {
		t.Fatal("expected different checksums for different content")
	}
}
