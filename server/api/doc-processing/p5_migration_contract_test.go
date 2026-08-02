package docprocessing

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func p5MigrationSQL(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../project_migrations", "20260801000015_add_p5_pipeline_predicates.sql"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestP5MigrationContract(t *testing.T) {
	sqlText := p5MigrationSQL(t)

	for _, fragment := range []string{
		"ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS name VARCHAR(128)",
		"ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS priority INT NOT NULL DEFAULT 0",
		"ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS active BOOLEAN NOT NULL DEFAULT true",
		"ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS tenant_id VARCHAR(128)",
		"ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS user_id VARCHAR(128)",
		"ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS input_record_id BIGINT",
		"ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS binding_kind TEXT NOT NULL DEFAULT 'store_default'",
		"ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS predicate JSONB",
		"ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS predicate_checksum TEXT",
		"ALTER TABLE kb.pipeline_bindings ADD COLUMN IF NOT EXISTS legacy_rule_id BIGINT",
		"ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS predicate JSONB",
		"ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS predicate_checksum TEXT",
		"ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS target_processor TEXT",
		"ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS effect TEXT",
		"ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS required_facets JSONB NOT NULL DEFAULT '[]'::jsonb",
		"ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS module_id TEXT",
		"ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS released_in_release_id BIGINT",
		"ALTER TABLE kb.pipeline_rules ADD COLUMN IF NOT EXISTS approval_status TEXT NOT NULL DEFAULT 'draft'",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("migration is missing required fragment %q", fragment)
		}
	}

	for _, fragment := range []string{
		"CHECK (binding_kind IN ('conditional', 'store_default'))",
		"CHECK ((predicate IS NULL) = (predicate_checksum IS NULL))",
		"CHECK (effect IN ('require', 'enable', 'skip', 'defer'))",
		"CHECK (approval_status IN ('draft', 'approved', 'included_in_release', 'rejected'))",
		"CHECK (NOT (legacy_rule_id IS NULL AND predicate IS NOT NULL AND binding_kind = 'store_default'))",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("migration is missing contract constraint %q", fragment)
		}
	}

	for _, fragment := range []string{
		"INSERT INTO kb.pipeline_bindings",
		"'conditional'",
		"legacy_rule_id",
		"jsonb_build_object(",
		"'version', 1",
		"'kind', 'all'",
		"r.match_input_doc_type",
		"r.match_source_language",
		"r.match_knowledge_store_binding",
		"md5(",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("migration is missing legacy selector copy fragment %q", fragment)
		}
	}

	for _, fragment := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_pipeline_bindings_legacy_rule_id",
		"CREATE INDEX IF NOT EXISTS idx_kb_pipeline_bindings_policy_active_priority_scope",
		"CREATE INDEX IF NOT EXISTS idx_kb_pipeline_rules_policy_target_processor_priority",
	} {
		if !strings.Contains(sqlText, fragment) {
			t.Fatalf("migration is missing index fragment %q", fragment)
		}
	}
}
