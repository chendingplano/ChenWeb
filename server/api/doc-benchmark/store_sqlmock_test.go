package docbenchmark

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
	"time"
)

func TestSQLStoreCreateCaseRunCanonicalJSON(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectQuery("INSERT INTO kb.benchmark_case_runs").WithArgs("r", "c", 1, "applicable", []byte(`{"x":1}`), (*string)(nil)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("id"))
	s := SQLStore{DB: db}
	if _, e := s.CreateCaseRun(context.Background(), "r", "c", 1, "applicable", map[string]int{"x": 1}, nil); e != nil {
		t.Fatal(e)
	}
	if e := m.ExpectationsWereMet(); e != nil {
		t.Fatal(e)
	}
}

func TestSQLStoreCreateCaseRunConflict(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectQuery("INSERT INTO kb.benchmark_case_runs").WithArgs("r", "c", 1, "applicable", []byte(`{"x":1}`), (*string)(nil)).WillReturnRows(sqlmock.NewRows([]string{"id"}))
	_, err := (SQLStore{DB: db}).CreateCaseRun(context.Background(), "r", "c", 1, "applicable", map[string]int{"x": 1}, nil)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLStoreOwnerXOR(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	s := SQLStore{DB: db}
	_, e := s.InsertScore(context.Background(), ScoreRecord{AttemptID: sql.NullString{String: "a", Valid: true}, RunID: sql.NullString{String: "r", Valid: true}})
	if e == nil {
		t.Fatal("expected owner xor error")
	}
	_ = m
}

func TestSQLStoreMarkVerifiedCASRejectsVerifiedMutation(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	marker := AllocationMarker{AttemptID: "a", Nonce: "n"}
	mh := markerDigest(marker)
	m.ExpectBegin()
	m.ExpectExec("UPDATE kb\\.benchmark_workspaces SET verified=true").WithArgs("a", "n", "new", int64(2), mh, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectRollback()
	err := (SQLStore{DB: db}).MarkVerifiedCAS(context.Background(), "a", "n", mh, "new", 2, marker)
	if err == nil {
		t.Fatal("expected CAS mutation rejection")
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLStoreInsertScoreIdempotent(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	r := ScoreRecord{AttemptID: sql.NullString{String: "a", Valid: true}, Processor: "p", Scorer: "s", ScorerVersion: "1", Metric: "m", Slice: "all", Direction: "higher", AggregationKind: "mean", Applicable: true, Metadata: json.RawMessage(`{"x":1}`)}
	m.ExpectBegin()
	m.ExpectQuery("SELECT case_run_id FROM kb.benchmark_case_attempts").WithArgs("a").WillReturnRows(sqlmock.NewRows([]string{"case_run_id"}).AddRow("cr"))
	m.ExpectQuery("SELECT lifecycle,run_id FROM kb.benchmark_case_runs").WithArgs("cr").WillReturnRows(sqlmock.NewRows([]string{"lifecycle", "run_id"}).AddRow("running", "r"))
	m.ExpectQuery("SELECT lifecycle FROM kb.benchmark_runs").WithArgs("r").WillReturnRows(sqlmock.NewRows([]string{"lifecycle"}).AddRow("running"))
	m.ExpectQuery("SELECT a.lifecycle,c.selected_attempt_id").WithArgs("a").WillReturnRows(sqlmock.NewRows([]string{"lifecycle", "selected_attempt_id"}).AddRow("running", nil))
	m.ExpectQuery("SELECT id,processor,scorer").WithArgs("a", nil, "m", "all", "mean").WillReturnRows(sqlmock.NewRows([]string{"id", "processor", "scorer", "scorer_version", "direction", "value", "additive_component", "numerator", "denominator", "non_null", "applicable", "metadata_json"}).AddRow("id", "p", "s", "1", "higher", nil, nil, nil, nil, false, true, []byte(`{"x":1}`)))
	m.ExpectCommit()
	id, err := (SQLStore{DB: db}).InsertScore(context.Background(), r)
	if err != nil || id != "id" {
		t.Fatalf("%s %v", id, err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSQLStoreInsertScoreConflict(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	r := ScoreRecord{RunID: sql.NullString{String: "r", Valid: true}, Processor: "p", Scorer: "s", ScorerVersion: "1", Metric: "m", Slice: "all", Direction: "higher", AggregationKind: "mean", Metadata: json.RawMessage(`{}`)}
	m.ExpectBegin()
	m.ExpectQuery("SELECT lifecycle FROM kb.benchmark_runs").WithArgs("r").WillReturnRows(sqlmock.NewRows([]string{"lifecycle"}).AddRow("running"))
	m.ExpectQuery("SELECT id,processor,scorer").WithArgs(nil, "r", "m", "all", "mean").WillReturnRows(sqlmock.NewRows([]string{"id", "processor", "scorer", "scorer_version", "direction", "value", "additive_component", "numerator", "denominator", "non_null", "applicable", "metadata_json"}).AddRow("id", "other", "s", "1", "higher", nil, nil, nil, nil, false, true, []byte(`{}`)))
	if _, err := (SQLStore{DB: db}).InsertScore(context.Background(), r); !IsConflict(err) {
		t.Fatalf("expected conflict, got %v", err)
	}
	m.ExpectRollback()
}

func TestFinishAttemptIdempotent(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectExec("UPDATE kb.benchmark_case_attempts SET lifecycle=").WithArgs("a", "owner", sqlmock.AnyArg(), "succeeded", "", int64(3), true).WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery("SELECT lifecycle,COALESCE\\(failure_kind").WithArgs("a").WillReturnRows(sqlmock.NewRows([]string{"lifecycle", "failure_kind"}).AddRow("succeeded", ""))
	if err := (SQLStore{DB: db}).FinishAttempt(context.Background(), "a", "owner", "succeeded", "", 3, true); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSelectAttemptIdempotent(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectExec("UPDATE kb.benchmark_case_runs SET selected_attempt_id=").WithArgs("run", "attempt").WillReturnResult(sqlmock.NewResult(0, 0))
	m.ExpectQuery("SELECT selected_attempt_id FROM kb.benchmark_case_runs").WithArgs("run").WillReturnRows(sqlmock.NewRows([]string{"selected_attempt_id"}).AddRow("attempt"))
	if err := (SQLStore{DB: db}).SelectAttempt(context.Background(), "run", "attempt"); err != nil {
		t.Fatal(err)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestReportScoresSelected(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectQuery("SELECT s.id,s.attempt_id,s.run_id").WithArgs("run").WillReturnRows(sqlmock.NewRows([]string{"id", "attempt_id", "run_id", "processor", "scorer", "scorer_version", "metric", "slice", "direction", "aggregation_kind", "value", "additive_component", "numerator", "denominator", "non_null", "applicable", "metadata_json"}).AddRow("s", "a", nil, "p", "sc", "1", "m", "all", "higher", "mean", nil, nil, nil, nil, false, true, []byte(`{}`)))
	out, err := (SQLStore{DB: db}).ReportScores(context.Background(), "run")
	if err != nil || len(out) != 1 {
		t.Fatalf("%v %#v", err, out)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestClaimAttemptLiveReturnsUnclaimed(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	m.ExpectBegin()
	m.ExpectQuery("SELECT lifecycle,selected_attempt_id").WithArgs("cr").WillReturnRows(sqlmock.NewRows([]string{"lifecycle", "selected_attempt_id", "max"}).AddRow("running", nil, 1))
	m.ExpectQuery("SELECT count\\(\\*\\)").WithArgs("cr", sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	m.ExpectCommit()
	c, err := (SQLStore{DB: db}).ClaimAttempt(context.Background(), "cr", "owner", time.Now(), time.Minute, 3)
	if err != nil || c.Claimed {
		t.Fatalf("%v %#v", err, c)
	}
	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

var _ driver.Value
