package kbhandler

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestTaskLLMExecutionPersistsFailedAllocation(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	previous := ApiTypes.SharedDBHandle
	ApiTypes.SharedDBHandle = nil
	t.Cleanup(func() { ApiTypes.SharedDBHandle = previous })
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs("kb.metric_score_runs/7").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT llm_run_id").WithArgs("kb.metric_score_runs/7").WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO kb.llm_executions").WithArgs("kb.metric_score_runs/7", int64(0)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
	mock.ExpectExec("UPDATE kb.metric_score_runs SET llm_run_id").WithArgs(int64(7), int64(0)).WillReturnResult(sqlmock.NewResult(0, 1))
	if got, err := taskLLMExecution(context.Background(), db, "kb.metric_score_runs", 7); got != 0 || err != nil {
		t.Fatalf("got %d", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func expectTaskLLMExecution(mock sqlmock.Sqlmock, table string, id int64) {
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT llm_run_id").WillReturnRows(sqlmock.NewRows([]string{"llm_run_id"}).AddRow(90000 + id))
	mock.ExpectCommit()
	mock.ExpectExec("UPDATE "+table+" SET llm_run_id").WithArgs(id, 90000+id).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestTaskBindingFailureStopsModelWork(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("binding unavailable"))
	_, err = executeScore(context.Background(), db, loggerutil.CreateDefaultLogger("20261108-698"), metricScoreKind, 7, metricScoreRequest{}, "user")
	if err == nil || !strings.Contains(err.Error(), "binding unavailable") {
		t.Fatalf("continued after binding failure: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
