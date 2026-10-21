package docprocessing

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joho/godotenv"
	"github.com/lib/pq"
)

func TestPythonSumyRunnerIntegration(t *testing.T) {
	python := os.Getenv("SUMY_TEST_PYTHON")
	if python == "" {
		t.Skip("set SUMY_TEST_PYTHON to the provisioned interpreter")
	}
	config := SumyConfig{Python: python, Algorithm: "lexrank", SentenceCount: 2, Timeout: 30 * time.Second}
	request := SumyRequest{Algorithm: "lexrank", SentenceCount: 2, Language: "mixed", Chunks: []SumyChunk{
		{SeqNo: 1, Lines: []SumyLine{{LineNumber: 10, LineType: "text", Content: "Store samples below 8°C. Keep the containers closed. Inspect them before shipment."}}},
		{SeqNo: 2, Lines: []SumyLine{{LineNumber: 20, LineType: "text", Content: "防护门打开时，不得启动电机。不得绕过安全联锁。设备应保持清洁。"}}},
	}}
	response, err := (PythonSumyRunner{Config: config}).Extract(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateSumyResponse(request, response); err != nil {
		t.Fatal(err)
	}
	if len(response.Summaries) != 2 || response.Summaries[0].Language != "en" || response.Summaries[1].Language != "zh" {
		t.Fatalf("%+v", response)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (PythonSumyRunner{Config: config}).Extract(ctx, request); err == nil {
		t.Fatal("cancelled worker succeeded")
	}
}

// Uses the project's configured PostgreSQL pool credentials but only creates an
// isolated temporary schema. Existing inputs/outputs and migration history are
// never modified. No password or DSN is printed or passed on the command line.
func TestExtractSummariesMigrationIntegration(t *testing.T) {
	if os.Getenv("EX_SUMMARY_DB_TEST") != "1" {
		t.Skip("set EX_SUMMARY_DB_TEST=1 for isolated-schema migration/store checks")
	}
	fileEnv, _ := godotenv.Read(filepath.Join("..", "..", "..", ".env"))
	for target, source := range map[string]string{"PGUSER": "PG_USER_NAME", "PGPASSWORD": "PG_PASSWORD", "PGDATABASE": "PG_DB_NAME"} {
		if os.Getenv(target) == "" {
			value := os.Getenv(source)
			if value == "" {
				value = fileEnv[source]
			}
			t.Setenv(target, value)
		}
	}
	if os.Getenv("PGHOST") == "" {
		t.Setenv("PGHOST", "127.0.0.1")
	}
	db, err := sql.Open("postgres", "sslmode=disable")
	if err != nil {
		t.Fatal("open PostgreSQL failed")
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("ex_summary_test_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, "CREATE SCHEMA "+pq.QuoteIdentifier(schema)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := db.ExecContext(context.Background(), "DROP SCHEMA "+pq.QuoteIdentifier(schema)+" CASCADE"); err != nil {
			t.Errorf("cleanup isolated schema: %v", err)
		}
	}()
	if _, err := db.ExecContext(ctx, "CREATE TABLE "+schema+".inputs (id bigint PRIMARY KEY); INSERT INTO "+schema+".inputs VALUES (42)"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "project_migrations", "20261021000002_create_kb_ex_summaries.sql"))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(raw), "-- +goose Down")
	if len(parts) != 2 {
		t.Fatal("missing down migration")
	}
	// Keep the already-installed summary trigger function; redirect only table
	// references to the isolated schema, exercising the actual current function.
	remap := strings.NewReplacer("kb.ex_summaries", schema+".ex_summaries", "REFERENCES kb.inputs", "REFERENCES "+schema+".inputs")
	up, down := remap.Replace(parts[0]), remap.Replace(parts[1])
	for _, statement := range []string{up, down, up, up} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	insert := "INSERT INTO " + schema + ".ex_summaries (summary_id,input_record_id,algorithm,summary_seq_no,summary_text) VALUES ($1,42,$2,1,$3)"
	for _, algorithm := range []string{"lexrank", "textrank"} {
		if _, err := db.ExecContext(ctx, insert, algorithm, algorithm, "source words"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(ctx, insert, "duplicate", "lexrank", "duplicate"); err == nil {
		t.Fatal("duplicate algorithm/chunk accepted")
	}
	var search string
	if err := db.QueryRowContext(ctx, "SELECT search_document FROM "+schema+".ex_summaries WHERE algorithm='lexrank'").Scan(&search); err != nil || search != "source words" {
		t.Fatalf("search trigger: %q %v", search, err)
	}
	if _, err := db.ExecContext(ctx, "DELETE FROM "+schema+".inputs WHERE id=42"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRowContext(ctx, "SELECT count(*) FROM "+schema+".ex_summaries").Scan(&count); err != nil || count != 0 {
		t.Fatalf("cascade rows=%d error=%v", count, err)
	}
}
