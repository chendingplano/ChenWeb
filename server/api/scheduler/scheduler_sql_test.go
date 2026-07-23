package scheduler

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSQLStoreDueSchedulesReadsRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery("FROM kb.scheduled_jobs").
		WithArgs(now, 50).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "job_type", "interval_seconds", "params", "enabled", "next_run_at",
		}).AddRow(
			int64(1), "Nightly Entity Resolve", "resolve_entity_objects", 3600, []byte(`{"limit":200}`), true, now,
		))

	store := SQLStore{DB: db}
	schedules, err := store.DueSchedules(context.Background(), now, 50)
	if err != nil {
		t.Fatalf("DueSchedules: %v", err)
	}
	if len(schedules) != 1 {
		t.Fatalf("schedules = %+v, want 1", schedules)
	}
	got := schedules[0]
	if got.ID != 1 || got.JobType != "resolve_entity_objects" || got.IntervalSeconds != 3600 || got.Params["limit"] != float64(200) {
		t.Fatalf("got %+v, unexpected shape", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSQLStoreStartRunInsertsRunningRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.scheduled_job_runs")).
		WithArgs(int64(1), "resolve_entity_objects", statusRunning).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(9)))

	store := SQLStore{DB: db}
	runID, err := store.StartRun(context.Background(), 1, "resolve_entity_objects")
	if err != nil {
		t.Fatalf("StartRun: %v", err)
	}
	if runID != 9 {
		t.Fatalf("runID = %d, want 9", runID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSQLStoreFinishRunWritesResultAndError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.scheduled_job_runs")).
		WithArgs("failed", `{"scanned":1}`, "boom", int64(9)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := SQLStore{DB: db}
	err = store.FinishRun(context.Background(), 9, "failed", map[string]any{"scanned": float64(1)}, errBoom)
	if err != nil {
		t.Fatalf("FinishRun: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

var errBoom = &boomError{}

type boomError struct{}

func (e *boomError) Error() string { return "boom" }

func TestSQLStoreAdvanceScheduleUpdatesNextRunAndStatus(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	next := time.Date(2026, 7, 23, 13, 0, 0, 0, time.UTC)
	mock.ExpectExec(regexp.QuoteMeta("UPDATE kb.scheduled_jobs")).
		WithArgs(next, "success", int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := SQLStore{DB: db}
	if err := store.AdvanceSchedule(context.Background(), 1, next, "success"); err != nil {
		t.Fatalf("AdvanceSchedule: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSQLStoreCreateScheduleInsertsRow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery(regexp.QuoteMeta("INSERT INTO kb.scheduled_jobs")).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(5)))

	store := SQLStore{DB: db}
	sched, err := store.CreateSchedule(context.Background(), CreateScheduleInput{
		Name: "Nightly Entity Resolve", JobType: "resolve_entity_objects", IntervalSeconds: 3600,
		Params: map[string]any{"limit": 200}, Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	if sched.ID != 5 {
		t.Fatalf("sched.ID = %d, want 5", sched.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSQLStoreListSchedulesReadsRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM kb.scheduled_jobs").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "name", "job_type", "interval_seconds", "params", "enabled",
			"next_run_at", "last_run_at", "last_run_status",
		}).AddRow(
			int64(1), "Nightly", "resolve_entity_objects", 3600, []byte(`{}`), true,
			time.Now(), nil, "",
		))

	store := SQLStore{DB: db}
	schedules, err := store.ListSchedules(context.Background())
	if err != nil {
		t.Fatalf("ListSchedules: %v", err)
	}
	if len(schedules) != 1 || schedules[0].Name != "Nightly" {
		t.Fatalf("schedules = %+v", schedules)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSQLStoreDeleteScheduleExecutesDelete(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM kb.scheduled_jobs")).
		WithArgs(int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	store := SQLStore{DB: db}
	if err := store.DeleteSchedule(context.Background(), 1); err != nil {
		t.Fatalf("DeleteSchedule: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestSQLStoreListRunsReadsHistory(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()

	started := time.Now()
	mock.ExpectQuery("FROM kb.scheduled_job_runs").
		WithArgs(int64(1), 50).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "schedule_id", "job_type", "status", "started_at", "finished_at", "result", "error",
		}).AddRow(
			int64(9), int64(1), "resolve_entity_objects", "success", started, started, []byte(`{"scanned":3}`), "",
		))

	store := SQLStore{DB: db}
	runs, err := store.ListRuns(context.Background(), 1, 50)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "success" || runs[0].Result["scanned"] != float64(3) {
		t.Fatalf("runs = %+v", runs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
