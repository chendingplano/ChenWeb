package classfoundation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMetricSupportCleanupMigrationPreservesEvidenceHistoryInAudit(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate metric support cleanup migration test")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations/20260818000017_create_metric_support_cleanup_audit.sql"))
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	compactSQL := strings.Join(strings.Fields(strings.ToLower(string(raw))), " ")
	for _, required := range []string{
		"create table if not exists kb.metric_support_cleanup_audit",
		"retained_evidence_id",
		"retired_evidence_id",
		"unique (retired_evidence_id)",
		"references kb.assertion_evidence(id)",
		"-- +goose down",
	} {
		if !strings.Contains(compactSQL, required) {
			t.Errorf("migration missing %q", required)
		}
	}
	if strings.Contains(compactSQL, "delete from kb.assertion_evidence") {
		t.Fatal("cleanup migration must not delete assertion evidence history")
	}
}
