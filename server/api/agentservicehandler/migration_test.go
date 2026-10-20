package agentservicehandler

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAgenticServiceMigrationEnforcesAttemptSourceIntegrity(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../project_migrations/20260914000001_create_agentic_service_tables.sql"))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(body)
	for _, required := range []string{
		"FOREIGN KEY (tool_call_id, attempt_id)",
		"REFERENCES kb.agentic_tool_calls(id, attempt_id)",
		"FOREIGN KEY (message_id, attempt_id)",
		"REFERENCES kb.agentic_messages(id, attempt_id)",
		"CHECK (BTRIM(source_fingerprint) <> '')",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing integrity clause %q", required)
		}
	}
}

func TestAgenticKnowledgeGrantMigrationIsUserAndStoreScoped(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../project_migrations/20260914000002_create_agentic_knowledge_grants.sql"))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(body)
	for _, required := range []string{"user_id", "knowledge_store_id", "document_id", "agentic_grant_document_store_fk", "active", "expires_at"} {
		if !strings.Contains(sql, required) {
			t.Fatalf("grant migration missing %q", required)
		}
	}
}

func TestAgenticRunGuardMigrationPreventsConcurrentAttempts(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../project_migrations/20260915000001_guard_agentic_active_attempts.sql"))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if !strings.Contains(string(body), "WHERE status = 'running'") || !strings.Contains(string(body), "UNIQUE INDEX") {
		t.Fatal("migration does not prevent two running attempts")
	}
}

func TestPiAgentProfileVersionsMigrationPreservesImmutableSnapshots(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not resolve test path")
	}
	path := filepath.Clean(filepath.Join(filepath.Dir(file), "../../../project_migrations/20261018000002_create_pi_agent_profile_versions.sql"))
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	// Column definitions are aligned with runs of spaces; compare on single spaces.
	sql := strings.Join(strings.Fields(string(body)), " ")
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS kb.agentic_service_profile_versions",
		"PRIMARY KEY (service_slug, version)",
		"profile_json JSONB NOT NULL",
		"prompt_ref TEXT NOT NULL",
		"prompt_hash TEXT NOT NULL",
		"prompt_snapshot TEXT NOT NULL",
		"created_by TEXT NOT NULL",
		"created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()",
		"CREATE OR REPLACE FUNCTION kb.reject_agentic_service_profile_version_mutation()",
		"agentic service profile versions are immutable",
		"BEFORE UPDATE OR DELETE ON kb.agentic_service_profile_versions",
		"CREATE TABLE IF NOT EXISTS kb.agentic_service_profiles",
		"active_version TEXT NOT NULL",
		"service_slug TEXT PRIMARY KEY",
		"FOREIGN KEY (service_slug, active_version)",
		"REFERENCES kb.agentic_service_profile_versions(service_slug, version)",
		"-- +goose Up",
		"-- +goose Down",
		"DROP TABLE IF EXISTS kb.agentic_service_profiles",
		"DROP TRIGGER IF EXISTS trg_agentic_service_profile_versions_immutable",
		"DROP FUNCTION IF EXISTS kb.reject_agentic_service_profile_version_mutation()",
		"DROP TABLE IF EXISTS kb.agentic_service_profile_versions",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("profile versions migration missing %q", required)
		}
	}
}
