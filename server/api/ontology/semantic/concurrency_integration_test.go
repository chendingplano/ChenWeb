package semantic

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/pressly/goose/v3"
)

// Task 4.1 / DR4 and DR10: two workers racing on the same occurrence and
// dependency must leave exactly one active outcome. The partial unique index
// is the guard, but a race that surfaces as a raw constraint error at a random
// call site is not the same as a race that resolves correctly -- hence the
// SELECT ... FOR UPDATE on the scope before the read in OutcomeStore.Record.
func TestIntegrationConcurrentWritersProduceOneActiveOutcome(t *testing.T) {
	db := freshSemanticTestDB(t)
	// A pool of one would serialize the workers and prove nothing.
	db.SetMaxOpenConns(8)
	store := OutcomeStore{DB: db}
	ctx := context.Background()

	const workers = 8
	var wg sync.WaitGroup
	errs := make([]error, workers)
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, errs[idx] = store.Record(ctx, baseOutcome("m-race"), nil)
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("worker %d: %v", i, err)
		}
	}
	key := OutcomeKey(1, "metric", "m-race", StageNormalize)
	var active, total int
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_outcomes WHERE outcome_key=$1 AND active`,
		key).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_outcomes WHERE outcome_key=$1`,
		key).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active outcomes after %d concurrent writers = %d, want exactly 1", workers, active)
	}
	// Identical inputs are an idempotent replay, so the losers must not have
	// appended history either.
	if total != 1 {
		t.Fatalf("total outcome rows = %d, want 1: identical concurrent writes are replays, not revisions", total)
	}
}

// Concurrent workers writing DIFFERENT dependencies must still converge on one
// active row, this time through supersession rather than replay.
func TestIntegrationConcurrentSupersessionKeepsOneActive(t *testing.T) {
	db := freshSemanticTestDB(t)
	db.SetMaxOpenConns(8)
	store := OutcomeStore{DB: db}
	ctx := context.Background()

	const workers = 6
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			out := baseOutcome("m-supersede")
			out.OutcomeCategory = CategorySemanticFinding
			out.DispositionTermID = DispositionRawPreserved
			fp := Dependencies{MappingRevision: string(rune('a' + idx))}.Fingerprint()
			<-start
			_, _ = store.Record(ctx, out, []Finding{mappingFinding(fp)})
		}(i)
	}
	close(start)
	wg.Wait()

	key := OutcomeKey(1, "metric", "m-supersede", StageNormalize)
	var active, orphanFindings int
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_processing_outcomes WHERE outcome_key=$1 AND active`,
		key).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`
SELECT count(*) FROM kb.semantic_processing_findings f
JOIN kb.semantic_processing_outcomes o ON o.id = f.outcome_id
WHERE f.active AND NOT o.active`).Scan(&orphanFindings); err != nil {
		t.Fatal(err)
	}
	if active != 1 {
		t.Fatalf("active outcomes = %d, want exactly 1", active)
	}
	if orphanFindings != 0 {
		t.Fatalf("%d active finding(s) survived under an inactive outcome", orphanFindings)
	}
}

// Task 4.10: the Phase 1 migrations are additive and roll back cleanly. A
// rollback must not delete committed raw-preserved rows, so this also asserts
// that pre-existing assertion data survives.
func TestIntegrationPhase1MigrationsRollBackCleanly(t *testing.T) {
	db := freshSemanticTestDB(t)
	_, thisFile, _, _ := runtime.Caller(0)
	migrations := filepath.Join(filepath.Dir(thisFile), "../../../../project_migrations")

	if _, err := db.Exec(`
INSERT INTO kb.semantic_assertions
  (logical_identity_key, subject_ref_kind, subject_ref_id, predicate_term_id,
   object_ref_kind, object_literal, status, value_state_term_id)
VALUES ('test:survives','object_node','obj-1','mea:measured_by','literal','{"value":1}'::jsonb,'represented',$1)`,
		ValuePresent); err != nil {
		t.Fatalf("seed assertion: %v", err)
	}

	// Roll back exactly the eight Phase 1 migrations.
	for i := 0; i < 8; i++ {
		if err := goose.Down(db, migrations); err != nil {
			t.Fatalf("goose down (step %d): %v", i+1, err)
		}
	}

	for _, table := range []string{
		"semantic_processing_outcomes", "semantic_processing_findings",
		"unresolved_semantic_occurrences", "semantic_retry_queue", "semantic_adapter_compliance",
	} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables
			WHERE table_schema='kb' AND table_name=$1`, table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("table kb.%s survived rollback", table)
		}
	}

	// The committed row must still be there: a rollback disables new writers,
	// it does not delete committed history (ADR section 6).
	var survived int
	if err := db.QueryRow(`SELECT count(*) FROM kb.semantic_assertions WHERE logical_identity_key='test:survives'`).
		Scan(&survived); err != nil {
		t.Fatal(err)
	}
	if survived != 1 {
		t.Fatalf("rollback deleted committed assertion data: found %d rows, want 1", survived)
	}

	// Re-applying must succeed, so a rollback is not one-way.
	if err := goose.Up(db, migrations); err != nil {
		t.Fatalf("re-apply after rollback: %v", err)
	}
	var readded int
	if err := db.QueryRow(`SELECT count(*) FROM information_schema.tables
		WHERE table_schema='kb' AND table_name='semantic_processing_outcomes'`).Scan(&readded); err != nil {
		t.Fatal(err)
	}
	if readded != 1 {
		t.Fatal("re-applying the Phase 1 migrations did not recreate the outcome table")
	}
	_ = sql.ErrNoRows
}
