package deployment

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func newMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, mock
}

func TestCreateBuildInsertsEveryPlannedStep(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO kb.deployment_builds`).
		WithArgs("20261116093000", "chenweb", "unknown", "linux/amd64", "desc", "admin@example.com", "mac").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(7))
	for i, p := range Plan(KindBuild) {
		mock.ExpectExec(`INSERT INTO kb.deployment_steps \(build_id, seq, step_key, step_arg\)`).
			WithArgs(int64(7), i+1, p.Key, p.Arg).WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectCommit()
	id, err := CreateBuild(context.Background(), db, Run{BuildNumber: "20261116093000", BuildName: "chenweb",
		TargetMachine: "unknown", OSInfo: "linux/amd64", Description: "desc", RequestedBy: "admin@example.com", RunnerHost: "mac"})
	if err != nil || id != 7 {
		t.Fatalf("CreateBuild = %d, %v", id, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateDeploymentReportsActiveRun(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectQuery(`INSERT INTO kb.deployments`).
		WillReturnError(errors.New(`pq: duplicate key value violates unique constraint "idx_deployments_one_active"`))
	mock.ExpectRollback()
	_, err := CreateDeployment(context.Background(), db, Run{ArchiveName: "runshen-deploy-1.tar.gz"})
	if !errors.Is(err, ErrActiveRun) {
		t.Fatalf("err = %v, want ErrActiveRun", err)
	}
}

func TestClaimDeploymentWithNothingQueued(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectQuery(`UPDATE kb.deployments SET status = 'running'`).WithArgs(42).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, ok, err := ClaimDeployment(context.Background(), db, 42)
	if ok || err != nil {
		t.Fatalf("ClaimDeployment = %v, %v", ok, err)
	}
}

func TestFailRunLeavesFinishedRunAlone(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE kb.deployment_builds SET status = 'failed'`).WithArgs(int64(3), ErrRunnerExited).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectRollback()
	if err := FailRun(context.Background(), db, KindBuild, 3, ErrRunnerExited); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFailRunClosesUnfinishedSteps(t *testing.T) {
	db, mock := newMock(t)
	mock.ExpectBegin()
	mock.ExpectExec(`UPDATE kb.deployments SET status = 'failed'`).WithArgs(int64(5), ErrRunnerExited).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE kb.deployment_steps SET finished_at = NOW\(\),\s+status = CASE WHEN status = 'running' THEN 'failed' ELSE 'skipped' END\s+WHERE deployment_id = \$1`).
		WithArgs(int64(5)).WillReturnResult(sqlmock.NewResult(0, 4))
	mock.ExpectCommit()
	if err := FailRun(context.Background(), db, KindDeploy, 5, ErrRunnerExited); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
