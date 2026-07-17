package docbenchmark

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestStoreAttachResolvedRuntimeAndFinalizeOnlyAfterAllCasesTerminal(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store := SQLStore{DB: db}
	mock.ExpectExec("UPDATE kb.benchmark_runs SET resolved_json=\\$2,config_json=\\$2,resolved_hash=\\$3,config_hash=\\$3").WithArgs("run", []byte(`{"chunk_size":100}`), "hash").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.AttachResolvedRuntime(context.Background(), "run", []byte(`{"chunk_size":100}`), "hash"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectExec("UPDATE kb.benchmark_runs SET lifecycle='running'").WithArgs("run", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := store.MarkRunRunning(context.Background(), "run"); err != nil {
		t.Fatal(err)
	}
	mock.ExpectQuery("SELECT count\\(\\*\\),count\\(\\*\\) FILTER").WithArgs("run").WillReturnRows(sqlmock.NewRows([]string{"total", "terminal"}).AddRow(2, 1))
	terminal, err := store.FinalizeRunIfComplete(context.Background(), "run")
	if err != nil || terminal {
		t.Fatalf("terminal=%v err=%v", terminal, err)
	}
	mock.ExpectQuery("SELECT count\\(\\*\\),count\\(\\*\\) FILTER").WithArgs("run").WillReturnRows(sqlmock.NewRows([]string{"total", "terminal"}).AddRow(2, 2))
	mock.ExpectExec("UPDATE kb.benchmark_runs SET lifecycle=CASE WHEN NOT EXISTS.*THEN 'succeeded'.*THEN 'canceled' ELSE 'failed' END").WithArgs("run", sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	terminal, err = store.FinalizeRunIfComplete(context.Background(), "run")
	if err != nil || !terminal {
		t.Fatalf("terminal=%v err=%v", terminal, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAttachRunProvenanceReturnsResumeConflictDetails(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store := SQLStore{DB: db}
	mock.ExpectExec("UPDATE kb.benchmark_runs SET git_commit=NULLIF\\(\\$2,''\\),jj_change=NULLIF\\(\\$3,''\\),executable=NULLIF\\(\\$4,''\\),executable_hash=NULLIF\\(\\$5,''\\),dirty=\\$6,concurrency=\\$7,updated_at=now\\(\\) WHERE id=\\$1 AND lifecycle IN \\('queued','running'\\) AND \\(executable_hash IS NULL OR \\(executable_hash=\\$5 AND dirty=\\$6\\)\\)").
		WithArgs("run", "git", "jj", "/tmp/bin", "new-hash", false, 2).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT lifecycle,dirty,executable_hash FROM kb.benchmark_runs WHERE id=\\$1").
		WithArgs("run").
		WillReturnRows(sqlmock.NewRows([]string{"lifecycle", "dirty", "executable_hash"}).AddRow("queued", true, "old-hash"))
	err := store.AttachRunProvenance(context.Background(), "run", "git", "jj", "/tmp/bin", "new-hash", false, 2)
	if err == nil {
		t.Fatal("expected provenance conflict")
	}
	msg := err.Error()
	for _, want := range []string{"cannot resume", "stored provenance", "dirty=true", "dirty=false", "start a fresh experiment"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
