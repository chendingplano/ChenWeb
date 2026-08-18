package classfoundation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAssertionClassReferenceMigrationUsesStateAwareConstraint(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate assertion class reference migration test")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations/20260818000014_add_assertion_class_references.sql"))
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	compactSQL := strings.Join(strings.Fields(strings.ToLower(string(raw))), " ")
	for _, required := range []string{
		"add column if not exists instance_of_term_id text",
		"references kb.ontology_term_headers (term_id)",
		"normalized_against_contract_revision_id",
		"references kb.ontology_class_contract_revisions (id)",
		"semantic:resolved_existing",
		"semantic:provisional_new",
		"semantic:ambiguous_candidates",
		"instance_of_term_id is not null",
		"idx_kb_semantic_assertions_instance_of_term",
		"-- +goose down",
	} {
		if !strings.Contains(compactSQL, required) {
			t.Errorf("migration missing %q", required)
		}
	}
}
