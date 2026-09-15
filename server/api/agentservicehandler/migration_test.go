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
