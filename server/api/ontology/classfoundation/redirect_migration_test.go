package classfoundation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRedirectMigrationEnforcesSingleActiveTargetsAndCycleGuards(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate redirect migration test")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations/20260818000016_create_semantic_redirects.sql"))
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	compactSQL := strings.Join(strings.Fields(strings.ToLower(string(raw))), " ")
	for _, required := range []string{
		"create table if not exists kb.ontology_term_redirects",
		"create table if not exists kb.semantic_assertion_redirects",
		"source_term_id",
		"target_term_id",
		"source_assertion_id",
		"target_assertion_id",
		"superseded_by_redirect_id",
		"where active",
		"create trigger kb_term_redirect_cycle_guard",
		"create trigger kb_assertion_redirect_cycle_guard",
		"recursive",
		"-- +goose down",
	} {
		if !strings.Contains(compactSQL, required) {
			t.Errorf("migration missing %q", required)
		}
	}
}
