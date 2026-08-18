package classfoundation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTermIdentityFoundationMigrationIsAdditiveAndBackfillsHistory(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate migration contract test")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations/20260818000009_create_ontology_term_identity_foundation.sql"))
	migration, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration %s: %v", migrationPath, err)
	}

	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS kb.ontology_term_headers",
		"CREATE TABLE IF NOT EXISTS kb.ontology_term_revisions",
		"INSERT INTO kb.ontology_term_headers",
		"INSERT INTO kb.ontology_term_revisions",
		"CREATE OR REPLACE VIEW kb.ontology_terms_current",
		"UNIQUE (term_id, revision)",
		"source_term_row_id",
	} {
		if !strings.Contains(string(migration), required) {
			t.Errorf("migration missing %q", required)
		}
	}

	for _, forbidden := range []string{
		"ALTER TABLE kb.ontology_terms",
		"DROP TABLE kb.ontology_terms",
	} {
		if strings.Contains(string(migration), forbidden) {
			t.Errorf("migration must retain legacy term table, found %q", forbidden)
		}
	}
}
