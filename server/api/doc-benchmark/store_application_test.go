package docbenchmark

import (
	"context"
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
