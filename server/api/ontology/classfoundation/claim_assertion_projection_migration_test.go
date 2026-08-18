package classfoundation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClaimAssertionProjectionRequiresLogicalKeyEqualClaimID(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate claim assertion projection migration test")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations/20260818000015_create_semantic_claim_assertion_projection.sql"))
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	compactSQL := strings.Join(strings.Fields(strings.ToLower(string(raw))), " ")
	for _, required := range []string{
		"create or replace view kb.semantic_claim_assertion_projection",
		"join kb.semantic_claim_identities as claim",
		"claim.claim_id = assertion.logical_identity_key",
		"assertion.logical_identity_key = claim.claim_id",
		"-- +goose down",
	} {
		if !strings.Contains(compactSQL, required) {
			t.Errorf("migration missing %q", required)
		}
	}
}
