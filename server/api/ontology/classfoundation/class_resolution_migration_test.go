package classfoundation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClassResolutionMigrationStoresAlternativesAndAppendOnlyHistory(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate class resolution migration test")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations/20260818000012_create_class_resolution_decisions.sql"))
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(raw))
	compactSQL := strings.Join(strings.Fields(sql), " ")
	for _, required := range []string{
		"create table if not exists kb.ontology_class_resolution_decisions",
		"create table if not exists kb.ontology_class_resolution_alternatives",
		"source_artifact_type",
		"source_artifact_id",
		"selected_class_term_id",
		"identity_state",
		"evidence jsonb",
		"supersedes_decision_id",
		"candidate_class_term_id",
		"candidate_key",
		"rank",
		"create trigger kb_class_resolution_decisions_immutable",
		"create trigger kb_class_resolution_alternatives_immutable",
		"-- +goose down",
	} {
		if !strings.Contains(compactSQL, required) {
			t.Errorf("migration missing %q", required)
		}
	}
}
