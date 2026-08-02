package docprocessing

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRoutingClearanceMigrationContract(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../project_migrations/20260801000017_create_kb_pipeline_routing_clearances.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(raw)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS kb.pipeline_routing_clearances",
		"CREATE TABLE IF NOT EXISTS kb.pipeline_routing_clearance_coverage",
		"CREATE TABLE IF NOT EXISTS kb.pipeline_routing_clearance_revocations",
		"policy_version", "document_kind", "manifest_checksum", "baseline_run_id", "routed_run_id",
		"subject_kind", "subject_checksum", "net_plan_delta_checksum", "superseded_clearance_id",
		"CHECK (subject_kind IN ('processor_rule', 'conditional_binding'))",
		"UNIQUE (policy_id, policy_version, subject_kind, subject_id, document_kind, clearance_id)",
		"TIMESTAMPTZ", "-- +goose Down",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q", required)
		}
	}
}
