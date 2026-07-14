package docbenchmark

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func benchmarkMigrationSQL(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Join(filepath.Dir(file), "../../../project_migrations")
	var b strings.Builder
	for _, name := range []string{
		"20260713000003_create_doc_benchmark_tables.sql",
		"20260713000004_extend_benchmark_workspace_ownership.sql",
	} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

func benchmarkMigrationFile(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../../../project_migrations", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestBenchmarkMigrationSQLMatchesPersistenceContract(t *testing.T) {
	sqlText := benchmarkMigrationSQL(t)
	for _, column := range []string{
		"input_record_id", "work_root", "evidence_path", "evidence_root",
		"verified", "verified_hash", "verified_size", "verified_marker_hash", "verified_marker",
	} {
		if !regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(column) + `\b`).MatchString(sqlText) {
			t.Errorf("migration is missing persistence column %s", column)
		}
	}
	for _, state := range []string{"pending", "active", "error", "db_pending", "files_pending", "cleaned"} {
		if !strings.Contains(sqlText, "'"+state+"'") {
			t.Errorf("cleanup_state does not accept %q", state)
		}
	}
	initialSchema := benchmarkMigrationFile(t, "20260713000003_create_doc_benchmark_tables.sql")
	if strings.Contains(initialSchema, "cancelled") {
		t.Error("initial schema uses cancelled; Runner lifecycle contract is canceled")
	}
	if !strings.Contains(sqlText, "REFERENCES kb.inputs(id) ON DELETE SET NULL") {
		t.Error("workspace input ownership must use ON DELETE SET NULL")
	}
	if !strings.Contains(sqlText, "OLD.input_record_id_snapshot IS NOT NULL AND OLD.input_record_id_snapshot IS DISTINCT FROM NEW.input_record_id_snapshot") {
		t.Error("attempt input snapshot guard must permit its initial NULL-to-value binding")
	}
}

func TestBenchmarkWorkspaceExtensionRepairsPreviouslyAppliedSchema(t *testing.T) {
	extension := benchmarkMigrationFile(t, "20260713000004_extend_benchmark_workspace_ownership.sql")
	for _, fragment := range []string{
		"DROP CONSTRAINT IF EXISTS benchmark_workspaces_cleanup_state_check",
		"cleanup_state IN ('pending','active','error','db_pending','files_pending','cleaned')",
		"DROP CONSTRAINT IF EXISTS benchmark_case_attempts_lifecycle_check",
		"lifecycle IN ('queued','leased','running','succeeded','failed','canceled')",
		"CREATE OR REPLACE FUNCTION kb.benchmark_terminal_guard()",
	} {
		if !strings.Contains(extension, fragment) {
			t.Errorf("extension does not repair existing schema: missing %q", fragment)
		}
	}
	updates := []struct {
		name, statement, addConstraint string
	}{
		{
			name:          "workspace cleanup state",
			statement:     "UPDATE kb.benchmark_workspaces SET cleanup_state='error' WHERE cleanup_state='failed'",
			addConstraint: "ADD CONSTRAINT benchmark_workspaces_cleanup_state_check",
		},
		{
			name:          "run lifecycle",
			statement:     "UPDATE kb.benchmark_runs SET lifecycle='canceled' WHERE lifecycle='cancelled'",
			addConstraint: "ADD CONSTRAINT benchmark_runs_lifecycle_check",
		},
		{
			name:          "attempt lifecycle",
			statement:     "UPDATE kb.benchmark_case_attempts SET lifecycle='canceled' WHERE lifecycle='cancelled'",
			addConstraint: "ADD CONSTRAINT benchmark_case_attempts_lifecycle_check",
		},
		{
			name:          "attempt failure kind",
			statement:     "UPDATE kb.benchmark_case_attempts SET failure_kind='canceled' WHERE failure_kind='cancelled'",
			addConstraint: "ADD CONSTRAINT ck_failure_kind",
		},
	}
	for _, update := range updates {
		t.Run(update.name, func(t *testing.T) {
			updateAt := strings.Index(extension, update.statement)
			constraintAt := strings.Index(extension, update.addConstraint)
			if updateAt < 0 {
				t.Fatalf("extension is missing data migration %q", update.statement)
			}
			if constraintAt < 0 || updateAt > constraintAt {
				t.Fatalf("data migration must precede %q", update.addConstraint)
			}
		})
	}
}

func TestOwnershipReadsNullableVerifiedSizeFromExtendedRows(t *testing.T) {
	columns := []string{"execution_attempt_id", "canonical_dir", "work_root", "evidence_path", "evidence_root", "nonce", "verified", "verified_hash", "verified_size", "verified_marker_hash", "cleanup_state"}
	row := func() *sqlmock.Rows {
		return sqlmock.NewRows(columns).AddRow("attempt", "/work", "/root", "/evidence/file", "/evidence", "nonce", false, nil, nil, nil, "active")
	}
	t.Run("load", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		mock.ExpectQuery("SELECT execution_attempt_id,canonical_dir,work_root").WithArgs("attempt").WillReturnRows(row())
		got, err := (SQLStore{DB: db}).LoadOwnership("attempt")
		if err != nil {
			t.Fatal(err)
		}
		if got.VerifiedSize != 0 || got.Verified {
			t.Fatalf("ownership=%+v", got)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("lock", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		mock.ExpectBegin()
		mock.ExpectQuery("SELECT execution_attempt_id,canonical_dir,work_root").WithArgs("attempt").WillReturnRows(row())
		mock.ExpectCommit()
		got, err := (SQLStore{DB: db}).LockOwnership("attempt")
		if err != nil {
			t.Fatal(err)
		}
		if got.VerifiedSize != 0 || got.Verified {
			t.Fatalf("ownership=%+v", got)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatal(err)
		}
	})
}

type inputOwnershipPersister interface {
	PersistInputOwnership(context.Context, string, int64) error
}

func TestPersistInputOwnershipBindsWorkspaceAndSnapshot(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	persister, ok := any(SQLStore{DB: db}).(inputOwnershipPersister)
	if !ok {
		t.Fatal("SQLStore does not implement PersistInputOwnership")
	}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT input_record_id FROM kb.benchmark_workspaces WHERE execution_attempt_id=$1 FOR UPDATE`)).
		WithArgs("attempt").
		WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}).AddRow(nil))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.benchmark_workspaces SET input_record_id=$2 WHERE execution_attempt_id=$1 AND input_record_id IS NULL`)).
		WithArgs("attempt", int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT input_record_id_snapshot FROM kb.benchmark_case_attempts WHERE id=$1 AND kind='execution' FOR UPDATE`)).
		WithArgs("attempt").
		WillReturnRows(sqlmock.NewRows([]string{"input_record_id_snapshot"}).AddRow(nil))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.benchmark_case_attempts SET input_record_id_snapshot=$2 WHERE id=$1 AND kind='execution' AND input_record_id_snapshot IS NULL`)).
		WithArgs("attempt", int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if err := persister.PersistInputOwnership(context.Background(), "attempt", 42); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPersistInputOwnershipIsIdempotentAndRejectsRebinding(t *testing.T) {
	for _, tc := range []struct {
		name      string
		workspace any
		snapshot  any
		wantErr   bool
	}{
		{name: "same value", workspace: int64(42), snapshot: int64(42)},
		{name: "workspace conflict", workspace: int64(41), snapshot: int64(42), wantErr: true},
		{name: "snapshot conflict", workspace: int64(42), snapshot: int64(41), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			persister, ok := any(SQLStore{DB: db}).(inputOwnershipPersister)
			if !ok {
				t.Fatal("SQLStore does not implement PersistInputOwnership")
			}
			mock.ExpectBegin()
			mock.ExpectQuery("SELECT input_record_id FROM kb\\.benchmark_workspaces").WithArgs("attempt").
				WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}).AddRow(tc.workspace))
			if tc.workspace == int64(42) {
				mock.ExpectQuery("SELECT input_record_id_snapshot FROM kb\\.benchmark_case_attempts").WithArgs("attempt").
					WillReturnRows(sqlmock.NewRows([]string{"input_record_id_snapshot"}).AddRow(tc.snapshot))
			}
			if tc.wantErr {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			err = persister.PersistInputOwnership(context.Background(), "attempt", 42)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v wantErr=%v", err, tc.wantErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCleanupTransactionDeletesOnlyOwnedProductionRowsInDependencyOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT input_record_id FROM kb.benchmark_workspaces WHERE execution_attempt_id=$1 FOR UPDATE`)).
		WithArgs("attempt").
		WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}).AddRow(int64(42)))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.metrics WHERE input_record_id=$1`)).
		WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.chunks WHERE source_record_id=$1`)).
		WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 3))
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM kb.inputs WHERE id=$1`)).
		WithArgs(int64(42)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	err = (SQLStore{DB: db}).CleanupTransaction(context.Background(), "attempt", func(tx CleanupTx) error {
		if err := tx.DeleteProductionRows(); err != nil {
			return err
		}
		return tx.DeleteInput()
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReportArtifactsFollowsVerifiedRescoreSource(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	query := `SELECT a.id,a.attempt_id,a.run_id,a.kind,a.path,a.sha256,a.size_bytes,a.verified,a.metadata_json FROM kb.benchmark_artifacts a LEFT JOIN kb.benchmark_case_attempts at ON at.id=a.attempt_id LEFT JOIN kb.benchmark_case_runs c ON c.id=at.case_run_id LEFT JOIN kb.benchmark_case_attempts selected ON selected.id=c.selected_attempt_id WHERE a.run_id=$1 OR (c.run_id=$1 AND (c.selected_attempt_id=a.attempt_id OR (selected.kind='rescore' AND selected.source_execution_attempt_id=a.attempt_id AND a.verified=true))) ORDER BY a.kind,a.path,a.id`
	mock.ExpectQuery(regexp.QuoteMeta(query)).WithArgs("run").WillReturnRows(sqlmock.NewRows([]string{
		"id", "attempt_id", "run_id", "kind", "path", "sha256", "size_bytes", "verified", "metadata_json",
	}).AddRow("artifact", "execution", nil, "actual", "result.json", "hash", int64(10), true, []byte(`{}`)))
	artifacts, err := (SQLStore{DB: db}).ReportArtifacts(context.Background(), "run")
	if err != nil {
		t.Fatal(err)
	}
	if len(artifacts) != 1 || artifacts[0].AttemptID != (sql.NullString{String: "execution", Valid: true}) {
		t.Fatalf("artifacts=%#v", artifacts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
