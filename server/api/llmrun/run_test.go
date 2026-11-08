package llmrun

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
)

func TestResolveReusesPersistedZero(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("queue/1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT llm_run_id FROM kb.llm_executions").WithArgs("queue/1").WillReturnRows(sqlmock.NewRows([]string{"llm_run_id"}).AddRow(0))
	mock.ExpectCommit()
	if got, err := Resolve(context.Background(), db, "queue/1"); got != 0 || err != nil {
		t.Fatalf("got %d", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestStampKeepsExecutionAcrossRedelivery(t *testing.T) {
	first, err := Stamp([]byte(`{"record_id":12}`))
	if err != nil {
		t.Fatal(err)
	}
	again, err := Stamp(first)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(again) {
		t.Fatalf("identity changed: %s -> %s", first, again)
	}
	if Key(first, 12) == Key(first, 13) {
		t.Fatal("batch documents share an execution")
	}
	rerun, _ := Stamp([]byte(`{"record_id":12}`))
	if Key(first, 12) == Key(rerun, 12) {
		t.Fatal("rerun reused execution")
	}
}

func TestInheritBatchExecution(t *testing.T) {
	parent, _ := Stamp([]byte(`{"record_ids":[12,13]}`))
	child, err := Inherit(parent, []byte(`{"record_id":12}`))
	if err != nil {
		t.Fatal(err)
	}
	if Key(parent, 12) != Key(child, 12) {
		t.Fatalf("batch identity lost: %s", child)
	}
}

func TestResolvePersistsAllocationFailureAndDoesNotRetry(t *testing.T) {
	previous := ApiTypes.SharedDBHandle
	ApiTypes.SharedDBHandle = nil
	t.Cleanup(func() { ApiTypes.SharedDBHandle = previous })
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("execution/1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT llm_run_id").WithArgs("execution/1").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO kb.llm_executions").WithArgs("execution/1", int64(0)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	if id, err := Resolve(context.Background(), db, "execution/1"); id != 0 || err != nil {
		t.Fatalf("got %d", id)
	}
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("execution/1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT llm_run_id").WithArgs("execution/1").WillReturnRows(sqlmock.NewRows([]string{"llm_run_id"}).AddRow(0))
	mock.ExpectCommit()
	if id, err := Resolve(context.Background(), db, "execution/1"); id != 0 || err != nil {
		t.Fatalf("redelivery got %d", id)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNextSharesCachedManagerAndSequence(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previous := ApiTypes.SharedDBHandle
	ApiTypes.SharedDBHandle = db
	t.Cleanup(func() { ApiTypes.SharedDBHandle = previous; managers.Delete(db) })
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT next_id, start_id").WithArgs("llm_run_id").WillReturnRows(sqlmock.NewRows([]string{"next_id", "start_id", "max_id", "allocate_size", "allow_reuse"}).AddRow(900, 1, 9999, 100, false))
	mock.ExpectExec("UPDATE shared.id_mgr").WithArgs(int64(1000), sqlmock.AnyArg(), "llm_run_id").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	for _, want := range []int64{900, 901} {
		if got := Next(context.Background()); got != want {
			t.Fatalf("got %d, want %d", got, want)
		}
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestQueueIdentityStableAcrossRedelivery(t *testing.T) {
	first := QueuePayload([]byte(`{"record_id":12}`), "documents", 39)
	again := QueuePayload([]byte(`{"record_id":12}`), "documents", 39)
	if Key(first, 12) != Key(again, 12) {
		t.Fatal("redelivery identity changed")
	}
	if Key(first, 12) == Key(QueuePayload([]byte(`{"record_id":12}`), "documents", 40), 12) {
		t.Fatal("different message reused identity")
	}
}

func TestResolvePersistenceFailureStopsExecution(t *testing.T) {
	for _, stage := range []string{"begin", "lookup", "commit"} {
		t.Run(stage, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			failure := errors.New("binding unavailable")
			if stage == "begin" {
				mock.ExpectBegin().WillReturnError(failure)
			} else {
				mock.ExpectBegin()
				mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
				query := mock.ExpectQuery("SELECT llm_run_id")
				if stage == "lookup" {
					query.WillReturnError(failure)
					mock.ExpectRollback()
				} else {
					query.WillReturnRows(sqlmock.NewRows([]string{"llm_run_id"}).AddRow(912))
					mock.ExpectCommit().WillReturnError(failure)
				}
			}
			worked := false
			id, err := Resolve(context.Background(), db, "queue/1")
			if err == nil {
				worked = true
			}
			if worked || id != 0 || !errors.Is(err, failure) {
				t.Fatalf("continued=%v id=%d err=%v", worked, id, err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProvisionPreservesHistoryAndReportsFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectExec("INSERT INTO shared.id_mgr").WillReturnResult(sqlmock.NewResult(0, 1))
	if err := Provision(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("bootstrap unavailable")
	mock.ExpectExec("INSERT INTO shared.id_mgr").WillReturnError(failure)
	if err := Provision(context.Background(), db); !errors.Is(err, failure) {
		t.Fatalf("got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
