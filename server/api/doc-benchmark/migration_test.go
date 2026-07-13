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
	for _, idx := range []string{"idx_benchmark_case_runs_selected", "idx_benchmark_scores_comparison", "uq_benchmark_scores_owner_metric", "uq_benchmark_case_runs_selected_once"} {
		var c int
		if err = db.QueryRow(`SELECT count(*) FROM pg_indexes WHERE schemaname='kb' AND indexname=$1`, idx).Scan(&c); err != nil || c != 1 {
			t.Fatalf("missing index %s", idx)
		}
	}
	for _, tbl := range []string{"benchmark_experiments", "benchmark_runs", "benchmark_case_runs", "benchmark_case_attempts", "benchmark_workspaces", "benchmark_scores", "benchmark_artifacts"} {
		var c int
		if err = db.QueryRow(`SELECT count(*) FROM pg_class WHERE relnamespace='kb'::regnamespace AND relname=$1`, tbl).Scan(&c); err != nil || c != 1 {
			t.Fatalf("missing catalog table %s", tbl)
		}
	}
	required := map[string][]string{"benchmark_experiments": {"id", "name", "raw_request_hash"}, "benchmark_runs": {"id", "experiment_id", "variant_name", "lifecycle"}, "benchmark_case_runs": {"id", "run_id", "case_id", "selected_attempt_id"}, "benchmark_case_attempts": {"id", "case_run_id", "attempt_number", "kind"}, "benchmark_workspaces": {"id", "execution_attempt_id", "canonical_dir"}, "benchmark_scores": {"id", "attempt_id", "run_id", "metric"}, "benchmark_artifacts": {"id", "attempt_id", "run_id", "sha256"}}
	for tbl, colsReq := range required {
		for _, col := range colsReq {
			var c int
			if err = db.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_schema='kb' AND table_name=$1 AND column_name=$2`, tbl, col).Scan(&c); err != nil || c != 1 {
				t.Fatalf("missing column %s.%s", tbl, col)
			}
		}
	}
	var cols int
	if err = db.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_schema='kb' AND table_name='benchmark_scores' AND column_name IN ('attempt_id','run_id','metric','slice','aggregation_kind')`).Scan(&cols); err != nil || cols != 5 {
		t.Fatalf("benchmark_scores catalog columns=%d err=%v", cols, err)
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
	err = db.QueryRow(`INSERT INTO kb.benchmark_case_runs(run_id,case_id,repetition,lifecycle) VALUES ($1,'c',1,'pending') RETURNING id`, run).Scan(&cr)
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
	if _, err = db.Exec(`INSERT INTO kb.benchmark_artifacts(attempt_id,run_id,kind,path,sha256,size_bytes) VALUES ($1,$2,'bad','z','h',1)`, at, run); err == nil {
		t.Fatal("artifact XOR owner check accepted")
	}
	if _, err = db.Exec(`INSERT INTO kb.benchmark_artifacts(attempt_id,kind,path,sha256,size_bytes) VALUES ($1,'log','x','h',1)`, at); err == nil {
		t.Fatal("artifact uniqueness check accepted")
	}
	if _, err = db.Exec(`UPDATE kb.benchmark_artifacts SET verified=true WHERE attempt_id=$1`, at); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE kb.benchmark_artifacts SET path='changed' WHERE attempt_id=$1`, at); err == nil {
		t.Fatal("verified artifact update accepted")
	}
	if _, err = db.Exec(`DELETE FROM kb.benchmark_artifacts WHERE attempt_id=$1`, at); err == nil {
		t.Fatal("verified artifact delete accepted")
	}
	var fk int
	if err = db.QueryRow(`SELECT count(*) FROM pg_constraint WHERE conrelid='kb.benchmark_workspaces'::regclass AND confrelid='kb.inputs'::regclass AND confdeltype='n'`).Scan(&fk); err != nil || fk != 1 {
		t.Fatalf("workspace input SET NULL FK missing: %v count=%d", err, fk)
	}
	if _, err = db.Exec(`UPDATE kb.benchmark_scores SET value=1 WHERE attempt_id=$1`, at); err == nil {
		t.Fatal("selected score update accepted")
	}
	if _, err = db.Exec(`DELETE FROM kb.benchmark_scores WHERE attempt_id=$1`, at); err == nil {
		t.Fatal("selected score delete accepted")
	}
	prod := map[string]bool{}
	for _, tbl := range []string{"inputs", "chunks", "metrics", "doc_proc_logs", "logs", "objects"} {
		var v sql.NullString
		if err = db.QueryRow(`SELECT to_regclass('kb.'||$1)`, tbl).Scan(&v); err != nil {
			t.Fatal(err)
		}
		prod[tbl] = v.Valid
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
	if err = db.QueryRow(`SELECT count(*) FROM pg_class WHERE relnamespace='kb'::regnamespace AND relname='inputs'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatal("down migration removed production kb.inputs")
	}
	for tbl, existed := range prod {
		if existed {
			var v sql.NullString
			if err = db.QueryRow(`SELECT to_regclass('kb.'||$1)`, tbl).Scan(&v); err != nil || !v.Valid {
				t.Fatalf("down removed production %s", tbl)
			}
		}
	}
	for _, tbl := range []string{"metrics", "logs", "objects"} {
		var exists sql.NullString
		if err = db.QueryRow(`SELECT to_regclass('kb.'||$1)`, tbl).Scan(&exists); err != nil {
			t.Fatal(err)
		}
	}
}
