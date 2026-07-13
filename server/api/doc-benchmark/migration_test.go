package docbenchmark

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

func TestBenchmarkMigrationIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.Ping(); err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "../../../project_migrations")
	goose.SetDialect("postgres")
	if err = goose.Up(db, dir); err != nil {
		t.Fatal(err)
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM pg_tables WHERE schemaname='kb' AND tablename LIKE 'benchmark_%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 7 {
		t.Fatalf("benchmark tables=%d, want 7", n)
	}
	// Minimal graph and representative constraints.
	var exp, run, cr, at string
	err = db.QueryRow(`INSERT INTO kb.benchmark_experiments(name,dataset_id,dataset_version,dataset_hash,raw_request_toml,raw_request_hash,resolved_experiment_json,resolved_file_hashes_json,resolved_case_set_json) VALUES ('x','d','v','h','x','rh','{}','{}','{}') RETURNING id`).Scan(&exp)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(`INSERT INTO kb.benchmark_runs(experiment_id,variant_name,lifecycle) VALUES ($1,'v','queued') RETURNING id`, exp).Scan(&run)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(`INSERT INTO kb.benchmark_case_runs(run_id,case_id,repetition,lifecycle) VALUES ($1,'c',1,'queued') RETURNING id`, run).Scan(&cr)
	if err != nil {
		t.Fatal(err)
	}
	err = db.QueryRow(`INSERT INTO kb.benchmark_case_attempts(case_run_id,attempt_number,kind,lifecycle) VALUES ($1,1,'execution','queued') RETURNING id`, cr).Scan(&at)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE kb.benchmark_case_runs SET selected_attempt_id=$1 WHERE id=$2`, at, cr); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO kb.benchmark_scores(attempt_id,processor,scorer,scorer_version,metric,slice,direction,aggregation_kind,applicable) VALUES ($1,'p','s','1','m','all','higher','mean',true)`, at); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE kb.benchmark_case_attempts SET lifecycle='succeeded' WHERE id=$1`, at); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE kb.benchmark_case_attempts SET lifecycle='running' WHERE id=$1`, at); err == nil {
		t.Fatal("terminal attempt update unexpectedly succeeded")
	}
	if _, err = db.Exec(`INSERT INTO kb.benchmark_runs(experiment_id,variant_name) VALUES ($1,'v')`, exp); err == nil {
		t.Fatal("duplicate run variant accepted")
	}
	if _, err = db.Exec(`INSERT INTO kb.benchmark_case_attempts(case_run_id,attempt_number,kind) VALUES ($1,1,'rescore')`, cr); err == nil {
		t.Fatal("rescore without source accepted")
	}
	var cr2, at2 string
	if err = db.QueryRow(`INSERT INTO kb.benchmark_case_runs(run_id,case_id,repetition) VALUES ($1,'other',1) RETURNING id`, run).Scan(&cr2); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`INSERT INTO kb.benchmark_case_attempts(case_run_id,attempt_number,kind) VALUES ($1,1,'execution') RETURNING id`, cr2).Scan(&at2); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE kb.benchmark_case_runs SET selected_attempt_id=$1 WHERE id=$2`, at2, cr); err == nil {
		t.Fatal("cross-case selected attempt accepted")
	}
	if _, err = db.Exec(`INSERT INTO kb.benchmark_workspaces(execution_attempt_id,canonical_dir,nonce) VALUES ($1,'/tmp/x','n')`, at); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO kb.benchmark_artifacts(attempt_id,kind,path,sha256,size_bytes) VALUES ($1,'log','x','h',1)`, at); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE kb.benchmark_scores SET value=1 WHERE attempt_id=$1`, at); err == nil {
		t.Fatal("selected score update accepted")
	}
	if _, err = db.Exec(`DELETE FROM kb.benchmark_scores WHERE attempt_id=$1`, at); err == nil {
		t.Fatal("selected score delete accepted")
	}
	if err = goose.Down(db, dir); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM pg_tables WHERE schemaname='kb' AND tablename LIKE 'benchmark_%'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("tables remain: %d", n)
	}
}
