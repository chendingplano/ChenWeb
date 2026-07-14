package docbenchmark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestSeedInputStagesExactBytesAndBindsInInsertTransaction(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	workspace := t.TempDir()
	body := []byte{0, 1, '\n', 0xff, 'x'}
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(seedInputQuery)).WithArgs("tenant", int64(44), "pdf", "case-a", "benchmark", filepath.Join(workspace, BenchmarkInputFilename), filepath.Join(workspace, BenchmarkInputFilename), BenchmarkInputFilename, "[]").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(91)))
	expectSeedBinding(mock, "attempt", 91)
	mock.ExpectCommit()
	id, err := SeedInput(context.Background(), db, SeedInputRequest{AttemptID: "attempt", Workspace: workspace, TenantID: "tenant", StoreID: 44, Title: "case-a", ParserName: "benchmark", Case: DatasetCase{InputBytes: body}})
	if err != nil || id != 91 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	got, err := os.ReadFile(filepath.Join(workspace, BenchmarkInputFilename))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("staged=%v want=%v", got, body)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSeedInputRollsBackInsertWhenBindingFails(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	workspace := t.TempDir()
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(seedInputQuery)).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(91)))
	mock.ExpectQuery("SELECT input_record_id FROM kb\\.benchmark_workspaces").WithArgs("attempt").WillReturnError(errors.New("forced bind failure"))
	mock.ExpectRollback()
	_, err := SeedInput(context.Background(), db, SeedInputRequest{AttemptID: "attempt", Workspace: workspace, TenantID: "tenant", StoreID: 44, Title: "case-a", ParserName: "benchmark", Case: DatasetCase{InputBytes: []byte("x")}})
	if err == nil {
		t.Fatal("expected error")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func expectSeedBinding(mock sqlmock.Sqlmock, attempt string, id int64) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT input_record_id FROM kb.benchmark_workspaces WHERE execution_attempt_id=$1 FOR UPDATE`)).WithArgs(attempt).WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}).AddRow(nil))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.benchmark_workspaces SET input_record_id=$2 WHERE execution_attempt_id=$1 AND input_record_id IS NULL`)).WithArgs(attempt, int64(id)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT input_record_id_snapshot FROM kb.benchmark_case_attempts WHERE id=$1 AND kind='execution' FOR UPDATE`)).WithArgs(attempt).WillReturnRows(sqlmock.NewRows([]string{"input_record_id_snapshot"}).AddRow(nil))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.benchmark_case_attempts SET input_record_id_snapshot=$2 WHERE id=$1 AND kind='execution' AND input_record_id_snapshot IS NULL`)).WithArgs(attempt, int64(id)).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestProductionArtifactPathMatchesRecordLayout(t *testing.T) {
	got, err := ProductionArtifactPath("/artifacts", 7523, "/stage/std_20039.pdf", "opendata", ".chunks")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/artifacts", "7", "7523", "std_20039_opendata.chunks")
	if got != want {
		t.Fatalf("got=%q want=%q", got, want)
	}
	metrics, err := ProductionArtifactPath("/artifacts", 162, "std_33830.pdf", "opendata", ".metrics")
	if err != nil || metrics != filepath.Join("/artifacts", "0", "162", "std_33830_opendata.metrics") {
		t.Fatalf("metrics=%q err=%v", metrics, err)
	}
}
