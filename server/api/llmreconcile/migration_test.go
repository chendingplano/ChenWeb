package llmreconcile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDualBillingMigrationDefinesAppendOnlyCurrencyScopedPersistence(t *testing.T) {
	path := filepath.Clean(filepath.Join("../../../project_migrations", "20260920000001_llm_usage_user_and_dual_billing.sql"))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	migration := string(raw)
	for _, want := range []string{
		"ADD COLUMN IF NOT EXISTS user_id TEXT NULL",
		"ALTER COLUMN balance_amount TYPE NUMERIC",
		"ADD COLUMN IF NOT EXISTS capture_source TEXT NOT NULL",
		"CREATE TABLE IF NOT EXISTS llm_balance_capture_slot",
		"UNIQUE (account_id, scheduled_hour)",
		"api_key_display_fingerprint",
		"CREATE TABLE IF NOT EXISTS llm_official_balance_delta_report",
		"CREATE TABLE IF NOT EXISTS llm_local_model_cost_report",
		"DROP TABLE IF EXISTS llm_local_model_cost_report",
		"DROP TABLE IF EXISTS llm_official_balance_delta_report",
	} {
		if !strings.Contains(migration, want) {
			t.Errorf("migration missing %q", want)
		}
	}
	if strings.Contains(migration, "UNIQUE (account_id, captured_at") {
		t.Fatal("snapshot rows must not have a uniqueness constraint")
	}
}
