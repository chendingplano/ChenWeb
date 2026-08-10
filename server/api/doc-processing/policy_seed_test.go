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

const pipelineVersionLockQuery = `SELECT id, version FROM kb.pipelines WHERE name = $1 ORDER BY version DESC LIMIT 1 FOR UPDATE`

const pipelineVersionInsertQuery = `
INSERT INTO kb.pipelines (name, display_name, description, processors, legacy_equivalent, is_system_default, version, status)
VALUES ($1, $2, $3, $4, false, $5, $6, 'active')
RETURNING id`

const pipelineRuleInsertQuery = `
INSERT INTO kb.pipeline_rules
    (name, priority, predicate, predicate_checksum, target_processor, effect, active, pipeline_id, approval_status)
VALUES ($1, 0, $2::jsonb, $3, $4, 'require', true, $5, 'approved')`

const pipelineBindingLookupByNameQuery = `SELECT id FROM kb.pipeline_bindings WHERE name = $1`

const pipelineBindingInsertQuery = `
INSERT INTO kb.pipeline_bindings (ks_store_id, pipeline_id, name, priority, active, binding_kind)
VALUES ($1, $2, $3, 0, true, 'store_default')`

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

	// authorDocProcessingPipelineVersion("all", ...) -- map iteration is made
	// deterministic by SeedDocProcessingPolicies sorting names, so "all" is
	// authored before "no-entities-relations". ValidatePipelineVersion runs
	// first and touches no DB (extract_entity/extract_metrics both only need
	// "chunks", produced by the baseline chunking processor).
	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionLockQuery)).
		WithArgs("all").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionInsertQuery)).
		WithArgs("all", "All", "All", sqlmock.AnyArg(), false, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	// processors sorted: extract_entity, extract_metrics
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("all: extract_entity", sqlmock.AnyArg(), sqlmock.AnyArg(), "extract_entity", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("all: extract_metrics", sqlmock.AnyArg(), sqlmock.AnyArg(), "extract_metrics", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// authorDocProcessingPipelineVersion("no-entities-relations", ...)
	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionLockQuery)).
		WithArgs("no-entities-relations").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionInsertQuery)).
		WithArgs("no-entities-relations", "No Entities Relations", "Default", sqlmock.AnyArg(), true, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	// processors sorted: extract_metrics, generate_topics
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("no-entities-relations: extract_metrics", sqlmock.AnyArg(), sqlmock.AnyArg(), "extract_metrics", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("no-entities-relations: generate_topics", sqlmock.AnyArg(), sqlmock.AnyArg(), "generate_topics", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// upsertDocProcessingBinding("system-default", nil, defaultPipelineID=1)
	mock.ExpectQuery(regexp.QuoteMeta(pipelineBindingLookupByNameQuery)).
		WithArgs("system-default").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(pipelineBindingInsertQuery)).
		WithArgs(nil, int64(1), "system-default").
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Research -> "all" (pipelineID=2)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.knowledge_store WHERE ks_name = $1`)).
		WithArgs("Research").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectQuery(regexp.QuoteMeta(pipelineBindingLookupByNameQuery)).
		WithArgs("store:Research").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(pipelineBindingInsertQuery)).
		WithArgs(int64(7), int64(2), "store:Research").
		WillReturnResult(sqlmock.NewResult(2, 1))

	mock.ExpectCommit()

	result, err := SeedDocProcessingPolicies(context.Background(), db, validSeedConfig())
	if err != nil {
		t.Fatalf("SeedDocProcessingPolicies: %v", err)
	}
	if len(result.PipelinesCreated) != 2 {
		t.Errorf("PipelinesCreated = %v, want 2 entries", result.PipelinesCreated)
	}
	if result.PipelineVersions["all"] != 1 || result.PipelineVersions["no-entities-relations"] != 1 {
		t.Errorf("PipelineVersions = %+v, want both at version 1 (first run)", result.PipelineVersions)
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

// TestSeedDocProcessingPoliciesSecondRunSupersedesAndUpsertsBindings proves
// re-running the tool authors a new version of an already-existing pipeline
// (superseding the prior one) and updates the existing bindings in place,
// rather than duplicating them (ADR 2026081001 DR1/DR3: bindings are
// upserted by name, not wholesale-replaced under a new policy generation).
func TestSeedDocProcessingPoliciesSecondRunSupersedesAndUpsertsBindings(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()

	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionLockQuery)).
		WithArgs("all").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}).AddRow(int64(2), 1))
	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionInsertQuery)).
		WithArgs("all", "All", "All", sqlmock.AnyArg(), false, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(22)))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.pipelines SET status = 'superseded', modify_time = NOW() WHERE id = $1`)).
		WithArgs(int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("all: extract_entity", sqlmock.AnyArg(), sqlmock.AnyArg(), "extract_entity", int64(22)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("all: extract_metrics", sqlmock.AnyArg(), sqlmock.AnyArg(), "extract_metrics", int64(22)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionLockQuery)).
		WithArgs("no-entities-relations").
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}).AddRow(int64(1), 1))
	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionInsertQuery)).
		WithArgs("no-entities-relations", "No Entities Relations", "Default", sqlmock.AnyArg(), true, 2).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(11)))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.pipelines SET status = 'superseded', modify_time = NOW() WHERE id = $1`)).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("no-entities-relations: extract_metrics", sqlmock.AnyArg(), sqlmock.AnyArg(), "extract_metrics", int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("no-entities-relations: generate_topics", sqlmock.AnyArg(), sqlmock.AnyArg(), "generate_topics", int64(11)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// The system-default binding already exists -> updated in place, not duplicated.
	mock.ExpectQuery(regexp.QuoteMeta(pipelineBindingLookupByNameQuery)).
		WithArgs("system-default").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(50)))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.pipeline_bindings SET pipeline_id = $1, active = true, modify_time = NOW() WHERE id = $2`)).
		WithArgs(int64(11), int64(50)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.knowledge_store WHERE ks_name = $1`)).
		WithArgs("Research").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectQuery(regexp.QuoteMeta(pipelineBindingLookupByNameQuery)).
		WithArgs("store:Research").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(51)))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.pipeline_bindings SET pipeline_id = $1, active = true, modify_time = NOW() WHERE id = $2`)).
		WithArgs(int64(22), int64(51)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	result, err := SeedDocProcessingPolicies(context.Background(), db, validSeedConfig())
	if err != nil {
		t.Fatalf("SeedDocProcessingPolicies: %v", err)
	}
	if len(result.PipelinesUpdated) != 2 || len(result.PipelinesCreated) != 0 {
		t.Errorf("PipelinesCreated=%v PipelinesUpdated=%v, want 0 created / 2 updated", result.PipelinesCreated, result.PipelinesUpdated)
	}
	if result.PipelineVersions["all"] != 2 || result.PipelineVersions["no-entities-relations"] != 2 {
		t.Errorf("PipelineVersions = %+v, want both at version 2 (second run)", result.PipelineVersions)
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

	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionLockQuery)).
		WithArgs("all").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionInsertQuery)).
		WithArgs("all", "All", "All", sqlmock.AnyArg(), false, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(2)))
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("all: extract_entity", sqlmock.AnyArg(), sqlmock.AnyArg(), "extract_entity", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("all: extract_metrics", sqlmock.AnyArg(), sqlmock.AnyArg(), "extract_metrics", int64(2)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionLockQuery)).
		WithArgs("no-entities-relations").WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(pipelineVersionInsertQuery)).
		WithArgs("no-entities-relations", "No Entities Relations", "Default", sqlmock.AnyArg(), true, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(1)))
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("no-entities-relations: extract_metrics", sqlmock.AnyArg(), sqlmock.AnyArg(), "extract_metrics", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(pipelineRuleInsertQuery)).
		WithArgs("no-entities-relations: generate_topics", sqlmock.AnyArg(), sqlmock.AnyArg(), "generate_topics", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectQuery(regexp.QuoteMeta(pipelineBindingLookupByNameQuery)).
		WithArgs("system-default").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(regexp.QuoteMeta(pipelineBindingInsertQuery)).
		WithArgs(nil, int64(1), "system-default").
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

// TestSeedDocProcessingPoliciesRejectsInvalidProcessorClosure proves
// ValidatePipelineVersion (ADR 2026081001 DR8) runs before any DB write --
// a config policy whose processors would fail closure is rejected with zero
// DB writes.
func TestSeedDocProcessingPoliciesRejectsInvalidProcessorClosure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	cfg := DocProcessingPolicySeedConfig{
		Policies: map[string]DocProcessingPolicySeedPolicy{
			"broken": {Description: "Broken", IsDefault: true, Processors: []string{"normalize_assertions"}},
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("cfg.Validate() failed unexpectedly: %v", err)
	}

	mock.ExpectBegin()
	mock.ExpectRollback()

	if _, err := SeedDocProcessingPolicies(context.Background(), db, cfg); err == nil {
		t.Fatal("expected a processor-closure validation error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations (no DB writes should occur before rollback): %v", err)
	}
}
