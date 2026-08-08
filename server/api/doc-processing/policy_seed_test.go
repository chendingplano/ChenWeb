// policy_seed_test.go

package docprocessing

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func validSeedConfig() DocProcessingPolicySeedConfig {
	return DocProcessingPolicySeedConfig{
		Policies: map[string]DocProcessingPolicySeedPolicy{
			"no-entities-relations": {Description: "Default", IsDefault: true, Processors: []string{"extract_metrics", "generate_topics"}},
			"all":                   {Description: "All", IsDefault: false, Processors: []string{"extract_metrics", "extract_entity"}},
		},
		Bindings: map[string]string{"Research": "all"},
	}
}

func TestSeedDocProcessingPolicies_NilDB(t *testing.T) {
	_, err := SeedDocProcessingPolicies(context.Background(), nil, validSeedConfig())
	if err == nil {
		t.Fatal("expected an error for a nil db")
	}
}

func TestSeedDocProcessingPolicies_InvalidConfig(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	_, err = SeedDocProcessingPolicies(context.Background(), db, DocProcessingPolicySeedConfig{})
	if err == nil {
		t.Fatal("expected an error for an invalid config")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("no DB calls should have been made: %v", err)
	}
}

func TestSeedDocProcessingPolicies_CreatesBothPipelinesOnFirstRun(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	// upsertPipeline("all") and upsertPipeline("no-entities-relations") -- map
	// iteration is made deterministic by SeedDocProcessingPolicies sorting
	// names, so "all" is upserted before "no-entities-relations".
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipelines WHERE name = $1`)).
		WithArgs("all").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipelines (name, display_name, description, processors, legacy_equivalent, is_system_default)`)).
		WithArgs("all", "All", "All", sqlmock.AnyArg(), false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipelines WHERE name = $1`)).
		WithArgs("no-entities-relations").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipelines (name, display_name, description, processors, legacy_equivalent, is_system_default)`)).
		WithArgs("no-entities-relations", "No Entities Relations", "Default", sqlmock.AnyArg(), true).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(version), 0) + 1 FROM kb.pipeline_policies`)).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipeline_policies (version, status, source_ref)`)).
		WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(30)))

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.pipeline_bindings`)).
		WithArgs(int64(1), int64(30)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.knowledge_store WHERE ks_name = $1`)).
		WithArgs("Research").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.pipeline_bindings`)).
		WithArgs(int64(7), int64(2), int64(30), "store:Research").
		WillReturnResult(sqlmock.NewResult(2, 1))

	// SeedDocProcessingPolicies also writes one unconditional (always-true
	// predicate, "require" effect) kb.pipeline_rules row per processor named
	// in each policy's processors list, iterating policy names then
	// processors both sorted -- "all" (extract_entity, extract_metrics)
	// before "no-entities-relations" (extract_metrics, generate_topics).
	for _, args := range [][2]string{
		{"all: extract_entity", "extract_entity"},
		{"all: extract_metrics", "extract_metrics"},
		{"no-entities-relations: extract_metrics", "extract_metrics"},
		{"no-entities-relations: generate_topics", "generate_topics"},
	} {
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.pipeline_rules`)).
			WithArgs(args[0], sqlmock.AnyArg(), sqlmock.AnyArg(), args[1], int64(30)).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}

	// PolicyCompilerSQLStore.CompilePolicy (policy_compile.go's loadDefinition)
	// issues, in order: version lookup, all pipelines, this policy's
	// bindings, this policy's gates (kb.pipeline_rules where predicate IS
	// NOT NULL -- none exist, since this seed tool never writes rules), and
	// routing-clearance coverage (also none). Exact query prefixes below are
	// copied from the real queries in policy_compile.go and from the
	// existing loadBindings test's row shape (policy_compile_test.go
	// TestPolicyCompilerSQLStoreLoadsLegacyAdapterForParity, 13 columns).
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT version FROM kb.pipeline_policies WHERE id=$1`)).
		WithArgs(int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT name,COALESCE(display_name,''),processors,legacy_equivalent FROM kb.pipelines`)).
		WillReturnRows(sqlmock.NewRows([]string{"name", "display_name", "processors", "legacy_equivalent"}).
			AddRow("all", "All", `{extract_metrics,extract_entity}`, false).
			AddRow("no-entities-relations", "Default", `{extract_metrics,generate_topics}`, false))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT b.id,COALESCE(b.name,''),b.priority,b.binding_kind,p.name`)).
		WithArgs(int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "priority", "kind", "pipeline", "predicate", "checksum", "active", "scope",
			"legacy_id", "doc_type", "language", "binding",
		}).
			AddRow(int64(1), "system-default", 0, "store_default", "no-entities-relations", "{}", "", true, "system", nil, "", "", "").
			AddRow(int64(2), "store:Research", 0, "store_default", "all", "{}", "", true, "knowledge_store", nil, "", "", ""))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id,name,priority,target_processor,effect,predicate::text,predicate_checksum,required_facets::text,active FROM kb.pipeline_rules`)).
		WithArgs(int64(30)).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "priority", "target_processor", "effect", "predicate", "predicate_checksum", "required_facets", "active",
		}))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT subject_kind,subject_id,subject_checksum FROM kb.pipeline_routing_clearance_coverage`)).
		WithArgs(int64(30), 3).
		WillReturnRows(sqlmock.NewRows([]string{"subject_kind", "subject_id", "subject_checksum"}))

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.pipeline_policies SET status = 'archived'`)).
		WithArgs(int64(30)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.pipeline_policies`)).
		WithArgs(sqlmock.AnyArg(), int64(30)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	result, err := SeedDocProcessingPolicies(context.Background(), db, validSeedConfig())
	if err != nil {
		t.Fatalf("SeedDocProcessingPolicies: %v", err)
	}
	if len(result.PipelinesCreated) != 2 {
		t.Errorf("PipelinesCreated = %v, want 2 entries", result.PipelinesCreated)
	}
	if result.PolicyID != 30 || result.PolicyVersion != 3 {
		t.Errorf("PolicyID/Version = %d/%d, want 30/3", result.PolicyID, result.PolicyVersion)
	}
	if result.BindingsWritten != 2 {
		t.Errorf("BindingsWritten = %d, want 2", result.BindingsWritten)
	}
	if result.RulesWritten != 4 {
		t.Errorf("RulesWritten = %d, want 4", result.RulesWritten)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestSeedDocProcessingPolicies_UnknownKnowledgeStoreRollsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipelines WHERE name = $1`)).
		WithArgs("all").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipelines (name, display_name, description, processors, legacy_equivalent, is_system_default)`)).
		WithArgs("all", "All", "All", sqlmock.AnyArg(), false).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipelines WHERE name = $1`)).
		WithArgs("no-entities-relations").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipelines (name, display_name, description, processors, legacy_equivalent, is_system_default)`)).
		WithArgs("no-entities-relations", "No Entities Relations", "Default", sqlmock.AnyArg(), true).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(version), 0) + 1 FROM kb.pipeline_policies`)).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(3))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipeline_policies (version, status, source_ref)`)).
		WithArgs(3).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(30)))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.pipeline_bindings`)).
		WithArgs(int64(1), int64(30)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.knowledge_store WHERE ks_name = $1`)).
		WithArgs("Research").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	_, err = SeedDocProcessingPolicies(context.Background(), db, validSeedConfig())
	if err == nil {
		t.Fatal("expected an error for an unknown knowledge store")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (rollback not observed?): %v", err)
	}
}
