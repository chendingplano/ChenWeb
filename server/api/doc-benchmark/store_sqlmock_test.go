package docbenchmark

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"github.com/DATA-DOG/go-sqlmock"
	"testing"
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

func TestSQLStoreInsertScoreIdempotent(t *testing.T) {
	db, m, _ := sqlmock.New()
	defer db.Close()
	r := ScoreRecord{AttemptID: sql.NullString{String: "a", Valid: true}, Processor: "p", Scorer: "s", ScorerVersion: "1", Metric: "m", Slice: "all", Direction: "higher", AggregationKind: "mean", Applicable: true, Metadata: json.RawMessage(`{"x":1}`)}
	m.ExpectQuery("SELECT id,processor,scorer").WithArgs("a", nil, "m", "all", "mean").WillReturnRows(sqlmock.NewRows([]string{"id", "processor", "scorer", "scorer_version", "direction", "value", "additive_component", "numerator", "denominator", "non_null", "applicable", "metadata_json"}).AddRow("id", "p", "s", "1", "higher", nil, nil, nil, nil, false, true, []byte(`{"x":1}`)))
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
	m.ExpectQuery("SELECT id,processor,scorer").WithArgs(nil, "r", "m", "all", "mean").WillReturnRows(sqlmock.NewRows([]string{"id", "processor", "scorer", "scorer_version", "direction", "value", "additive_component", "numerator", "denominator", "non_null", "applicable", "metadata_json"}).AddRow("id", "other", "s", "1", "higher", nil, nil, nil, nil, false, true, []byte(`{}`)))
	if _, err := (SQLStore{DB: db}).InsertScore(context.Background(), r); !IsConflict(err) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

var _ driver.Value
