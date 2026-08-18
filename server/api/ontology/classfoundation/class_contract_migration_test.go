package classfoundation

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClassContractMigrationProvidesAppendOnlyCapabilityEvidence(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate class contract migration test")
	}
	migrationPath := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations/20260818000010_create_ontology_class_contracts.sql"))
	raw, err := os.ReadFile(migrationPath)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := strings.ToLower(string(raw))
	compactSQL := strings.Join(strings.Fields(sql), " ")
	for _, required := range []string{
		"create table if not exists kb.ontology_class_contract_revisions",
		"create table if not exists kb.ontology_class_contract_capabilities",
		"create table if not exists kb.ontology_class_capability_validation_results",
		"unique (term_id, revision)",
		"contract_schema_version",
		"identity_schema_version",
		"definition_state",
		"contract_payload jsonb",
		"capability_term_id",
		"validator_id",
		"validator_version",
		"validation_result",
		"evidence jsonb",
		"references kb.ontology_term_headers (term_id)",
		"current_contract_revision_id",
		"-- +goose down",
	} {
		if !strings.Contains(compactSQL, required) {
			t.Errorf("migration missing %q", required)
		}
	}
}
