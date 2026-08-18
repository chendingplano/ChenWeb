package classfoundation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMetricSupportCardinalityMigrationRejectsOnlyCurrentMetricSupports(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate metric support cardinality migration test")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations/20260818000018_add_metric_support_cardinality.sql"))
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	compactSQL := strings.Join(strings.Fields(strings.ToLower(string(raw))), " ")
	for _, required := range []string{
		"create unique index if not exists uq_assertion_evidence_current_metric_support",
		"on kb.assertion_evidence (artifact_type, artifact_id, input_record_id) nulls not distinct",
		"where artifact_type = 'metric'",
		"evidence_role = 'supports'",
		"not deleted",
		"-- +goose down",
		"drop index if exists kb.uq_assertion_evidence_current_metric_support",
	} {
		if !strings.Contains(compactSQL, required) {
			t.Errorf("migration missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"where evidence_role = 'supports' and not deleted",
		"on kb.assertion_evidence (artifact_id, input_record_id)",
	} {
		if strings.Contains(compactSQL, forbidden) {
			t.Errorf("metric-only cardinality index is too broad: found %q", forbidden)
		}
	}
}
