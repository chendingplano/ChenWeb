package docbenchmark

import (
	"context"
	"database/sql"
	"database/sql/driver"
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

var _ driver.Value
