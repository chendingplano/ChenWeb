package classfoundation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClaimIdentityMigrationRegistersVersionsAndUniqueCanonicalKeys(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate claim identity migration test")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations/20260818000013_create_semantic_claim_identities.sql"))
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	compactSQL := strings.Join(strings.Fields(strings.ToLower(string(raw))), " ")
	for _, required := range []string{
		"create table if not exists kb.semantic_canonical_key_versions",
		"create table if not exists kb.semantic_claim_identities",
		"key_version text primary key",
		"claim_id text primary key",
		"canonical_key bytea not null",
		"unique (key_version, canonical_key)",
		"class_term_id",
		"identity_payload jsonb",
		"-- +goose down",
	} {
		if !strings.Contains(compactSQL, required) {
			t.Errorf("migration missing %q", required)
		}
	}
}
