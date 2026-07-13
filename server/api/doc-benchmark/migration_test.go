package docbenchmark

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	_ "github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

type benchmarkColumn struct{ typ, udt, nullable string }

func assertBenchmarkCatalog(t *testing.T, db *sql.DB) {
	t.Helper()
	N, Y := "NO", "YES"
	col := func(typ, udt, nullable string) benchmarkColumn { return benchmarkColumn{typ, udt, nullable} }
	expected := map[string]map[string]benchmarkColumn{
		"benchmark_experiments":   {"id": col("uuid", "uuid", N), "name": col("text", "text", N), "dataset_id": col("text", "text", N), "dataset_version": col("text", "text", N), "dataset_hash": col("text", "text", N), "raw_request_toml": col("text", "text", N), "raw_request_hash": col("text", "text", N), "resolved_experiment_json": col("jsonb", "jsonb", N), "resolved_file_hashes_json": col("jsonb", "jsonb", N), "resolved_case_set_json": col("jsonb", "jsonb", N), "created_at": col("timestamp with time zone", "timestamptz", N), "updated_at": col("timestamp with time zone", "timestamptz", N)},
		"benchmark_runs":          {"id": col("uuid", "uuid", N), "experiment_id": col("uuid", "uuid", N), "variant_name": col("text", "text", N), "lifecycle": col("text", "text", N), "requested_json": col("jsonb", "jsonb", N), "resolved_json": col("jsonb", "jsonb", N), "config_json": col("jsonb", "jsonb", N), "prompt_json": col("jsonb", "jsonb", N), "scorer_json": col("jsonb", "jsonb", N), "pricing_json": col("jsonb", "jsonb", N), "requested_hash": col("text", "text", Y), "resolved_hash": col("text", "text", Y), "config_hash": col("text", "text", Y), "prompt_hash": col("text", "text", Y), "scorer_hash": col("text", "text", Y), "pricing_hash": col("text", "text", Y), "git_commit": col("text", "text", Y), "jj_change": col("text", "text", Y), "executable": col("text", "text", Y), "executable_hash": col("text", "text", Y), "dirty": col("boolean", "bool", N), "concurrency": col("integer", "int4", Y), "usage_json": col("jsonb", "jsonb", N), "runtime_json": col("jsonb", "jsonb", N), "created_at": col("timestamp with time zone", "timestamptz", N), "updated_at": col("timestamp with time zone", "timestamptz", N), "started_at": col("timestamp with time zone", "timestamptz", Y), "finished_at": col("timestamp with time zone", "timestamptz", Y)},
		"benchmark_case_runs":     {"id": col("uuid", "uuid", N), "run_id": col("uuid", "uuid", N), "case_id": col("text", "text", N), "repetition": col("integer", "int4", N), "applicability": col("text", "text", N), "tags_json": col("jsonb", "jsonb", N), "upstream_hash": col("text", "text", Y), "lifecycle": col("text", "text", N), "selected_attempt_id": col("uuid", "uuid", Y), "created_at": col("timestamp with time zone", "timestamptz", N), "updated_at": col("timestamp with time zone", "timestamptz", N)},
		"benchmark_case_attempts": {"id": col("uuid", "uuid", N), "case_run_id": col("uuid", "uuid", N), "attempt_number": col("integer", "int4", N), "kind": col("text", "text", N), "source_execution_attempt_id": col("uuid", "uuid", Y), "input_record_id_snapshot": col("bigint", "int8", Y), "lifecycle": col("text", "text", N), "failure_kind": col("text", "text", Y), "lease_owner": col("text", "text", Y), "lease_expires_at": col("timestamp with time zone", "timestamptz", Y), "heartbeat_at": col("timestamp with time zone", "timestamptz", Y), "started_at": col("timestamp with time zone", "timestamptz", Y), "finished_at": col("timestamp with time zone", "timestamptz", Y), "runtime_ms": col("bigint", "int8", Y), "telemetry_json": col("jsonb", "jsonb", N), "provider": col("text", "text", Y), "model": col("text", "text", Y), "capture_verified": col("boolean", "bool", N), "created_at": col("timestamp with time zone", "timestamptz", N)},
		"benchmark_workspaces":    {"id": col("uuid", "uuid", N), "execution_attempt_id": col("uuid", "uuid", N), "input_record_id": col("bigint", "int8", Y), "canonical_dir": col("text", "text", N), "nonce": col("text", "text", N), "cleanup_state": col("text", "text", N), "cleanup_error": col("text", "text", Y), "created_at": col("timestamp with time zone", "timestamptz", N), "cleaned_at": col("timestamp with time zone", "timestamptz", Y)},
		"benchmark_scores":        {"id": col("uuid", "uuid", N), "attempt_id": col("uuid", "uuid", Y), "run_id": col("uuid", "uuid", Y), "processor": col("text", "text", N), "scorer": col("text", "text", N), "scorer_version": col("text", "text", N), "metric": col("text", "text", N), "slice": col("text", "text", N), "direction": col("text", "text", N), "aggregation_kind": col("text", "text", N), "value": col("numeric", "numeric", Y), "additive_component": col("numeric", "numeric", Y), "numerator": col("numeric", "numeric", Y), "denominator": col("numeric", "numeric", Y), "non_null": col("boolean", "bool", N), "applicable": col("boolean", "bool", N), "metadata_json": col("jsonb", "jsonb", N), "created_at": col("timestamp with time zone", "timestamptz", N)},
		"benchmark_artifacts":     {"id": col("uuid", "uuid", N), "attempt_id": col("uuid", "uuid", Y), "run_id": col("uuid", "uuid", Y), "kind": col("text", "text", N), "path": col("text", "text", N), "sha256": col("text", "text", N), "size_bytes": col("bigint", "int8", N), "verified": col("boolean", "bool", N), "metadata_json": col("jsonb", "jsonb", N), "created_at": col("timestamp with time zone", "timestamptz", N)},
	}
	for table, want := range expected {
		rows, err := db.Query(`SELECT column_name,data_type,udt_name,is_nullable FROM information_schema.columns WHERE table_schema='kb' AND table_name=$1 ORDER BY ordinal_position`, table)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]benchmarkColumn{}
		for rows.Next() {
			var n, typ, udt, nullable string
			if err := rows.Scan(&n, &typ, &udt, &nullable); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			got[n] = benchmarkColumn{typ, udt, nullable}
		}
		rows.Close()
		if len(got) != len(want) {
			t.Fatalf("%s columns=%d want %d", table, len(got), len(want))
		}
		for name, exp := range want {
			if act, ok := got[name]; !ok || act != exp {
				t.Errorf("%s.%s catalog=%v want=%v", table, name, act, exp)
			}
		}
	}

	constraints := map[string]struct{ typ, def string }{
		"uq_benchmark_experiments_request_hash": {"u", "UNIQUE (raw_request_hash)"},
		"uq_benchmark_runs_variant":             {"u", "UNIQUE (experiment_id, variant_name)"},
		"uq_benchmark_case_runs":                {"u", "UNIQUE (run_id, case_id, repetition)"},
		"uq_benchmark_case_attempts":            {"u", "UNIQUE (case_run_id, attempt_number)"},
		"uq_benchmark_case_attempt_owner":       {"u", "UNIQUE (id, case_run_id)"},
		"fk_attempt_source_same_case":           {"f", "FOREIGN KEY (source_execution_attempt_id, case_run_id) REFERENCES kb.benchmark_case_attempts(id, case_run_id)"},
		"ck_rescore_source":                     {"c", "CHECK (((kind = 'execution'::text) AND (source_execution_attempt_id IS NULL)) OR ((kind = 'rescore'::text) AND (source_execution_attempt_id IS NOT NULL)))"},
		"ck_failure_kind":                       {"c", "CHECK (((lifecycle <> ALL (ARRAY['failed'::text])) AND (failure_kind IS NULL)) OR (lifecycle = 'failed'::text))"},
		"fk_case_runs_selected_attempt":         {"f", "FOREIGN KEY (selected_attempt_id, id) REFERENCES kb.benchmark_case_attempts(id, case_run_id) DEFERRABLE INITIALLY DEFERRED"},
		"ck_score_owner_xor":                    {"c", "CHECK (((attempt_id IS NOT NULL) <> (run_id IS NOT NULL)))"},
		"ck_artifact_owner_xor":                 {"c", "CHECK (((attempt_id IS NOT NULL) <> (run_id IS NOT NULL)))"},
	}
	for name, exp := range constraints {
		var typ, def string
		err := db.QueryRow(`SELECT contype,pg_get_constraintdef(oid) FROM pg_constraint WHERE conname=$1`, name).Scan(&typ, &def)
		if err != nil {
			t.Errorf("constraint %s: %v", name, err)
			continue
		}
		if typ != exp.typ || strings.Join(strings.Fields(def), " ") != strings.Join(strings.Fields(exp.def), " ") {
			t.Errorf("constraint %s=(%s,%s), want (%s,%s)", name, typ, def, exp.typ, exp.def)
		}
	}

	indexes := map[string]struct {
		unique bool
		def    string
	}{"idx_benchmark_case_runs_selected": {false, "CREATE INDEX idx_benchmark_case_runs_selected ON kb.benchmark_case_runs USING btree (selected_attempt_id)"}, "uq_benchmark_case_runs_selected_once": {true, "CREATE UNIQUE INDEX uq_benchmark_case_runs_selected_once ON kb.benchmark_case_runs USING btree (selected_attempt_id) WHERE (selected_attempt_id IS NOT NULL)"}, "idx_benchmark_attempts_lifecycle_lease": {false, "CREATE INDEX idx_benchmark_attempts_lifecycle_lease ON kb.benchmark_case_attempts USING btree (lifecycle, lease_expires_at)"}, "idx_benchmark_attempts_diagnostics": {false, "CREATE INDEX idx_benchmark_attempts_diagnostics ON kb.benchmark_case_attempts USING btree (failure_kind, provider, model)"}, "idx_benchmark_runs_comparisons": {false, "CREATE INDEX idx_benchmark_runs_comparisons ON kb.benchmark_runs USING btree (experiment_id, lifecycle, variant_name)"}, "idx_benchmark_workspaces_cleanup": {false, "CREATE INDEX idx_benchmark_workspaces_cleanup ON kb.benchmark_workspaces USING btree (cleanup_state, cleaned_at)"}, "idx_benchmark_scores_attempt": {false, "CREATE INDEX idx_benchmark_scores_attempt ON kb.benchmark_scores USING btree (attempt_id)"}, "idx_benchmark_scores_run": {false, "CREATE INDEX idx_benchmark_scores_run ON kb.benchmark_scores USING btree (run_id)"}, "idx_benchmark_scores_comparison": {false, "CREATE INDEX idx_benchmark_scores_comparison ON kb.benchmark_scores USING btree (processor, metric, aggregation_kind, slice)"}, "idx_benchmark_artifacts_attempt": {false, "CREATE INDEX idx_benchmark_artifacts_attempt ON kb.benchmark_artifacts USING btree (attempt_id)"}, "idx_benchmark_artifacts_run": {false, "CREATE INDEX idx_benchmark_artifacts_run ON kb.benchmark_artifacts USING btree (run_id)"}, "uq_benchmark_scores_owner_metric": {true, "CREATE UNIQUE INDEX uq_benchmark_scores_owner_metric ON kb.benchmark_scores USING btree (COALESCE(attempt_id, run_id), (attempt_id IS NULL), metric, slice, aggregation_kind)"}, "uq_benchmark_artifacts_owner_kind_path": {true, "CREATE UNIQUE INDEX uq_benchmark_artifacts_owner_kind_path ON kb.benchmark_artifacts USING btree (COALESCE(attempt_id, run_id), (attempt_id IS NULL), kind, path)"}}
	for name, exp := range indexes {
		var unique bool
		var def string
		err := db.QueryRow(`SELECT i.indisunique,pg_get_indexdef(i.indexrelid) FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='kb' AND c.relname=$1`, name).Scan(&unique, &def)
		if err != nil {
			t.Errorf("index %s: %v", name, err)
			continue
		}
		if unique != exp.unique || strings.Join(strings.Fields(def), " ") != strings.Join(strings.Fields(exp.def), " ") {
			t.Errorf("index %s=(%v,%s), want (%v,%s)", name, unique, def, exp.unique, exp.def)
		}
	}
}

func expectExecError(t *testing.T, db *sql.DB, stmt string, args ...any) {
	t.Helper()
	if _, err := db.Exec(stmt, args...); err == nil {
		t.Fatalf("expected statement to fail: %s", stmt)
	}
}

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
	assertBenchmarkCatalog(t, db)
	for _, idx := range []string{"idx_benchmark_case_runs_selected", "idx_benchmark_scores_comparison", "uq_benchmark_scores_owner_metric", "uq_benchmark_case_runs_selected_once"} {
		var c int
		if err = db.QueryRow(`SELECT count(*) FROM pg_indexes WHERE schemaname='kb' AND indexname=$1`, idx).Scan(&c); err != nil || c != 1 {
			t.Fatalf("missing index %s", idx)
		}
	}
	var idef string
	if err = db.QueryRow(`SELECT indexdef FROM pg_indexes WHERE schemaname='kb' AND indexname='uq_benchmark_case_runs_selected_once'`).Scan(&idef); err != nil || !strings.Contains(idef, "WHERE (selected_attempt_id IS NOT NULL)") {
		t.Fatalf("selected partial index predicate missing: %v", err)
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
	if _, err = db.Exec(`INSERT INTO kb.benchmark_workspaces(execution_attempt_id,canonical_dir,nonce) VALUES ($1,'/tmp/kind','kind')`, at2); err != nil {
		t.Fatal(err)
	}
	expectExecError(t, db, `UPDATE kb.benchmark_case_attempts SET kind='rescore',source_execution_attempt_id=$1 WHERE id=$2`, at, at2)
	// Provenance: source must exist in the same case and be an execution attempt.
	var rs string
	if err = db.QueryRow(`INSERT INTO kb.benchmark_case_attempts(case_run_id,attempt_number,kind,source_execution_attempt_id) VALUES ($1,2,'rescore',$2) RETURNING id`, cr, at).Scan(&rs); err != nil {
		t.Fatal(err)
	}
	expectExecError(t, db, `INSERT INTO kb.benchmark_case_attempts(case_run_id,attempt_number,kind,source_execution_attempt_id) VALUES ($1,3,'rescore',$2)`, cr, rs)
	expectExecError(t, db, `INSERT INTO kb.benchmark_case_attempts(case_run_id,attempt_number,kind,source_execution_attempt_id) VALUES ($1,3,'rescore',$2)`, cr, "00000000-0000-0000-0000-000000000000")
	expectExecError(t, db, `INSERT INTO kb.benchmark_case_attempts(case_run_id,attempt_number,kind,source_execution_attempt_id) VALUES ($1,2,'rescore',$2)`, cr2, at)
	// Named CHECK constraints and owner XORs.
	expectExecError(t, db, `INSERT INTO kb.benchmark_case_runs(run_id,case_id,repetition) VALUES ($1,'bad',0)`, run)
	expectExecError(t, db, `INSERT INTO kb.benchmark_case_attempts(case_run_id,attempt_number,kind) VALUES ($1,0,'execution')`, cr)
	expectExecError(t, db, `INSERT INTO kb.benchmark_case_attempts(case_run_id,attempt_number,kind) VALUES ($1,9,'bogus')`, cr)
	expectExecError(t, db, `INSERT INTO kb.benchmark_scores(processor,scorer,scorer_version,metric,slice,direction,aggregation_kind) VALUES ('p','s','1','bad','all','higher','mean')`)
	expectExecError(t, db, `INSERT INTO kb.benchmark_scores(attempt_id,run_id,processor,scorer,scorer_version,metric,slice,direction,aggregation_kind) VALUES ($1,$2,'p','s','1','bad2','all','higher','mean')`, at, run)
	expectExecError(t, db, `INSERT INTO kb.benchmark_artifacts(attempt_id,kind,path,sha256,size_bytes) VALUES ($1,'bad','neg','h',-1)`, at)
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
