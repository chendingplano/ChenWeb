package llmrun

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	_ "github.com/lib/pq"
)

// Requires a disposable database, separate from the shared allocator tests.
func TestPostgresExecutionMigrationAndRedelivery(t *testing.T) {
	dsn := os.Getenv("LLMRUN_TEST_DSN")
	if dsn == "" {
		t.Skip("set LLMRUN_TEST_DSN to an isolated idmanager_test_* database")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sharedDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sharedDB.Close()
	db.SetMaxOpenConns(8)
	sharedDB.SetMaxOpenConns(8)
	var name string
	if err := db.QueryRow(`SELECT current_database()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "idmanager_test_") {
		t.Fatal("requires disposable idmanager_test_* database")
	}
	previous := ApiTypes.SharedDBHandle
	ApiTypes.SharedDBHandle = sharedDB
	t.Cleanup(func() { ApiTypes.SharedDBHandle = previous; managers.Delete(sharedDB) })
	exec := func(query string) {
		t.Helper()
		if _, err := db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	exec(`DROP SCHEMA IF EXISTS kb CASCADE; DROP SCHEMA IF EXISTS shared CASCADE; DROP TABLE IF EXISTS public.llm_usage_event;
 CREATE SCHEMA kb; CREATE SCHEMA IF NOT EXISTS testbed; DROP TABLE IF EXISTS testbed.generated_question_runs; DROP TABLE IF EXISTS testbed.provision_score_runs;
 CREATE TABLE testbed.generated_question_runs(id bigint PRIMARY KEY);
 CREATE TABLE kb.metric_score_runs(id bigint PRIMARY KEY); CREATE TABLE kb.reference_score_runs(id bigint PRIMARY KEY);
 CREATE TABLE testbed.provision_score_runs(id bigint PRIMARY KEY); CREATE TABLE kb.summary_score_runs(id bigint PRIMARY KEY);
 CREATE TABLE kb.doc_process_runs(id bigint PRIMARY KEY); CREATE TABLE kb.doc_review_runs(id bigint PRIMARY KEY);
 CREATE TABLE public.llm_usage_event(run_id bigint); INSERT INTO public.llm_usage_event VALUES (NULL),(5),(9400)`)
	sharedMigration, err := os.ReadFile("../../../../shared/go/api/goose/shared_migrations/20261108000001_id_manager.sql")
	if err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../../project_migrations/20261108000001_shared_llm_run_ids.sql")
	if err != nil {
		t.Fatal(err)
	}
	up := strings.Split(string(migration), "-- +goose Down")[0]
	exec(up)
	exec(strings.Split(string(sharedMigration), "-- +goose Down")[0])
	if err := Provision(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	var start int64
	if err := db.QueryRow(`SELECT start_id FROM shared.id_mgr WHERE id_name='llm_run_id'`).Scan(&start); err != nil {
		t.Fatal(err)
	}
	if start != 9401 {
		t.Fatalf("migration start=%d, want 9401", start)
	}
	const workers = 20
	ids := make(chan int64, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			id, err := Resolve(context.Background(), db, "delivery/record")
			if err != nil {
				t.Error(err)
			}
			ids <- id
		})
	}
	wg.Wait()
	close(ids)
	var original int64
	for id := range ids {
		if id <= 9400 {
			t.Fatalf("ID %d overlaps legacy", id)
		}
		if original == 0 {
			original = id
		}
		if id != original {
			t.Fatalf("redelivery changed ID %d -> %d", original, id)
		}
	}
	if got, err := Resolve(context.Background(), db, "rerun/record"); err != nil || got == original || got == 0 {
		t.Fatalf("rerun ID=%d, previous=%d", got, original)
	}
	// Repeated startup must preserve allocator progress.
	exec(up)
	if err := Provision(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got, err := Resolve(context.Background(), db, "delivery/record"); err != nil || got != original {
		t.Fatalf("repeated migration changed execution %d", got)
	}
	exec(strings.Split(string(migration), "-- +goose Down")[1])
	exec(up)
	if err := Provision(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if got := Next(context.Background()); got <= original {
		t.Fatalf("rollback reset sequence: %d", got)
	}
}
