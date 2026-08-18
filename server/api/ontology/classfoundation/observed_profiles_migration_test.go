package classfoundation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestObservedProfilesMigrationKeepsEvidenceSeparateFromContracts(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate observed profile migration test")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations/20260818000011_create_observed_class_profiles.sql"))
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(raw))
	for _, required := range []string{
		"create table if not exists kb.ontology_observed_class_profiles",
		"create table if not exists kb.ontology_observed_class_attribute_observations",
		"create table if not exists kb.ontology_observed_class_attribute_distributions",
		"create table if not exists kb.ontology_observed_class_profile_examples",
		"create table if not exists kb.ontology_observed_class_profile_exceptions",
		"class_term_id",
		"assertion_id",
		"evidence_id",
		"observation_state",
		"document_count",
		"outlier",
		"contradiction",
		"references kb.ontology_term_headers (term_id)",
		"-- +goose down",
	} {
		if !strings.Contains(sql, required) {
			t.Errorf("migration missing %q", required)
		}
	}
	if strings.Contains(sql, "ontology_class_contract") {
		t.Error("observed profile storage must not link to or modify authoritative contracts")
	}
}
