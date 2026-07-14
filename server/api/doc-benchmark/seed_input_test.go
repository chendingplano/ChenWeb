package docbenchmark

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
)

func TestSeedInputStagesExactBytesAndBindsInInsertTransaction(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	workspace := t.TempDir()
	workspace, _ = filepath.EvalSymlinks(workspace)
	body := []byte{0, 1, '\n', 0xff, 'x'}
	mock.ExpectBegin()
	const expectedSeedQuery = `INSERT INTO kb.inputs (tenant_id, ks_store_id, type, title, parser_name, staging_filename, result_filename, file_name, status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb) RETURNING id`
	stagingMetadata := filepath.Join(workspace, BenchmarkInputFilename)
	linePath := filepath.Join(workspace, "benchmark-input_benchmark.txt")
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT input_record_id FROM kb.benchmark_workspaces WHERE execution_attempt_id=$1 FOR UPDATE`)).WithArgs("attempt").WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}).AddRow(nil))
	mock.ExpectQuery(regexp.QuoteMeta(expectedSeedQuery)).WithArgs("tenant", int64(44), "pdf", "case-a", "benchmark", stagingMetadata, linePath, BenchmarkInputFilename, "[]").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(91)))
	expectSeedBinding(mock, "attempt", 91)
	mock.ExpectCommit()
	seeded, err := SeedInput(context.Background(), db, SeedInputRequest{AttemptID: "attempt", Workspace: workspace, TenantID: "tenant", StoreID: 44, Title: "case-a", ParserName: "benchmark", Case: DatasetCase{InputBytes: body}})
	if err != nil || seeded.ID != 91 {
		t.Fatalf("seeded=%#v err=%v", seeded, err)
	}
	resolved, err := docprocessing.ResolveInputFilePath(docprocessing.LineFileGeneratedEvent{}, seeded.ResultFilename, seeded.ParserName, seeded.StagingFilename)
	if err != nil {
		t.Fatalf("ResolveInputFilePath: %v", err)
	}
	got, err := os.ReadFile(resolved)
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
	workspace, _ = filepath.EvalSymlinks(workspace)
	mock.ExpectBegin()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT input_record_id FROM kb.benchmark_workspaces WHERE execution_attempt_id=$1 FOR UPDATE`)).WithArgs("attempt").WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}).AddRow(nil))
	mock.ExpectQuery(`INSERT INTO kb\.inputs \(tenant_id, ks_store_id, type, title, parser_name, staging_filename, result_filename, file_name, status\) VALUES \(\$1,\$2,\$3,\$4,\$5,\$6,\$7,\$8,\$9::jsonb\) RETURNING id`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(91)))
	mock.ExpectExec("UPDATE kb\\.benchmark_workspaces").WillReturnError(errors.New("forced bind failure"))
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
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.benchmark_workspaces SET input_record_id=$2 WHERE execution_attempt_id=$1 AND input_record_id IS NULL`)).WithArgs(attempt, int64(id)).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT input_record_id_snapshot FROM kb.benchmark_case_attempts WHERE id=$1 AND kind='execution' FOR UPDATE`)).WithArgs(attempt).WillReturnRows(sqlmock.NewRows([]string{"input_record_id_snapshot"}).AddRow(nil))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE kb.benchmark_case_attempts SET input_record_id_snapshot=$2 WHERE id=$1 AND kind='execution' AND input_record_id_snapshot IS NULL`)).WithArgs(attempt, int64(id)).WillReturnResult(sqlmock.NewResult(0, 1))
}

func TestSeedInputRetryPreservesWinner(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   []byte
		wantErr bool
	}{{"same", []byte("winner"), false}, {"conflict", []byte("loser"), true}} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			workspace := t.TempDir()
			workspace, _ = filepath.EvalSymlinks(workspace)
			linePath := filepath.Join(workspace, "benchmark-input_benchmark.txt")
			_ = os.WriteFile(linePath, []byte("winner"), 0o600)
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT input_record_id FROM kb\.benchmark_workspaces`).WithArgs("attempt").WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}).AddRow(int64(91)))
			mock.ExpectQuery(`SELECT tenant_id, ks_store_id, parser_name, staging_filename, result_filename, file_name, status::text FROM kb\.inputs`).WithArgs(int64(91)).WillReturnRows(sqlmock.NewRows([]string{"tenant_id", "ks_store_id", "parser_name", "staging_filename", "result_filename", "file_name", "status"}).AddRow("tenant", int64(44), "benchmark", filepath.Join(workspace, BenchmarkInputFilename), linePath, BenchmarkInputFilename, "[]"))
			if tc.wantErr {
				mock.ExpectRollback()
			} else {
				mock.ExpectCommit()
			}
			seeded, err := SeedInput(context.Background(), db, SeedInputRequest{AttemptID: "attempt", Workspace: workspace, TenantID: "tenant", StoreID: 44, ParserName: "benchmark", Case: DatasetCase{InputBytes: tc.input}})
			if (err != nil) != tc.wantErr {
				t.Fatalf("seeded=%#v err=%v", seeded, err)
			}
			got, _ := os.ReadFile(linePath)
			if string(got) != "winner" {
				t.Fatalf("winner changed: %q", got)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
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

func TestSeedAndArtifactPathsRejectEscape(t *testing.T) {
	workspace := t.TempDir()
	for _, parser := range []string{"../evil", "a/b", `a\\b`, ".", ".."} {
		if _, err := SeedInput(context.Background(), nil, SeedInputRequest{Workspace: workspace, ParserName: parser}); err == nil || !strings.Contains(err.Error(), "parser") {
			t.Fatalf("parser=%q err=%v", parser, err)
		}
		if _, err := ProductionArtifactPath(t.TempDir(), 1, "x.pdf", parser, ".chunks"); err == nil {
			t.Fatalf("artifact parser accepted %q", parser)
		}
	}
	db, _, _ := sqlmock.New()
	defer db.Close()
	_, err := SeedInput(context.Background(), db, SeedInputRequest{AttemptID: "a", Workspace: workspace, TenantID: "t", StoreID: 1, ParserName: "safe", ResultFilename: filepath.Join(workspace, "..", "outside.txt")})
	if err == nil || !strings.Contains(err.Error(), "workspace") {
		t.Fatalf("outside result err=%v", err)
	}
}

func TestSeedAndArtifactPathsRejectSymlinkDescendant(t *testing.T) {
	workspace, outside := t.TempDir(), t.TempDir()
	workspace, _ = filepath.EvalSymlinks(workspace)
	if err := os.Symlink(outside, filepath.Join(workspace, "linked")); err != nil {
		t.Fatal(err)
	}
	db, _, _ := sqlmock.New()
	defer db.Close()
	_, err := SeedInput(context.Background(), db, SeedInputRequest{AttemptID: "a", Workspace: workspace, TenantID: "t", StoreID: 1, ParserName: "safe", ResultFilename: filepath.Join(workspace, "linked", "missing.txt")})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("seed symlink err=%v", err)
	}
	artifactRoot := t.TempDir()
	_ = os.Symlink(outside, filepath.Join(artifactRoot, "0"))
	if _, err := ProductionArtifactPath(artifactRoot, 1, "x.pdf", "safe", ".chunks"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("artifact symlink err=%v", err)
	}
}

func TestSeedInputAdoptsMatchingOrphanAndPreservesFileOnCommitError(t *testing.T) {
	for _, commitErr := range []error{nil, errors.New("ambiguous commit")} {
		t.Run(fmt.Sprint(commitErr), func(t *testing.T) {
			db, mock, _ := sqlmock.New()
			defer db.Close()
			workspace := t.TempDir()
			workspace, _ = filepath.EvalSymlinks(workspace)
			linePath := filepath.Join(workspace, "benchmark-input_benchmark.txt")
			body := []byte("recoverable")
			_ = os.WriteFile(linePath, body, 0o600)
			mock.ExpectBegin()
			mock.ExpectQuery(`SELECT input_record_id FROM kb\.benchmark_workspaces`).WillReturnRows(sqlmock.NewRows([]string{"input_record_id"}).AddRow(nil))
			mock.ExpectQuery(`INSERT INTO kb\.inputs`).WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(92)))
			expectSeedBinding(mock, "attempt", 92)
			if commitErr == nil {
				mock.ExpectCommit()
			} else {
				mock.ExpectCommit().WillReturnError(commitErr)
			}
			_, err := SeedInput(context.Background(), db, SeedInputRequest{AttemptID: "attempt", Workspace: workspace, TenantID: "tenant", StoreID: 44, ParserName: "benchmark", Case: DatasetCase{InputBytes: body}})
			if (err != nil) != (commitErr != nil) {
				t.Fatalf("err=%v", err)
			}
			got, readErr := os.ReadFile(linePath)
			if readErr != nil || string(got) != string(body) {
				t.Fatalf("file lost: %q err=%v", got, readErr)
			}
		})
	}
}
