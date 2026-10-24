package kbhandler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
	"github.com/pressly/goose/v3"
)

// Opt in with QUESTION_GENERATOR_DB_TEST=1 under mise. Only a uniquely named
// scratch database is changed; the configured application database is untouched.
func TestQuestionGeneratorDatabaseLifecycle(t *testing.T) {
	if os.Getenv("QUESTION_GENERATOR_DB_TEST") != "1" {
		t.Skip("set QUESTION_GENERATOR_DB_TEST=1 for isolated PostgreSQL integration test")
	}
	for target, source := range map[string]string{"PGHOST": "PG_HOST", "PGPORT": "PG_PORT", "PGUSER": "PG_USER_NAME", "PGPASSWORD": "PG_PASSWORD"} {
		if v := strings.TrimSpace(os.Getenv(source)); v != "" {
			t.Setenv(target, v)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := sql.Open("postgres", "dbname=postgres sslmode=disable")
	if err != nil {
		t.Fatal("open PostgreSQL admin connection failed")
	}
	defer admin.Close()
	name := fmt.Sprintf("question_gen_test_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE "+pq.QuoteIdentifier(name)); err != nil {
		t.Fatalf("create scratch database: %v", err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.ExecContext(cleanup, "DROP DATABASE "+pq.QuoteIdentifier(name)+" WITH (FORCE)"); err != nil {
			t.Errorf("drop scratch database: %v", err)
		}
	}()
	db, err := sql.Open("postgres", "dbname="+name+" sslmode=disable")
	if err != nil {
		t.Fatal("open scratch database failed")
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA kb; CREATE TABLE kb.inputs (id BIGINT PRIMARY KEY); INSERT INTO kb.inputs VALUES (416)`); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join("../../../project_migrations", "20261024000001_create_generated_questions.sql"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "20261024000001_create_generated_questions.sql"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("migration up: %v", err)
	}
	if _, err := provider.Down(ctx); err != nil {
		t.Fatalf("migration down: %v", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("migration re-up: %v", err)
	}
	var id int64
	if err := db.QueryRowContext(ctx, `INSERT INTO testbed.generated_question_runs
  (input_record_id,purpose,lang,model_ref,prompt_name,one_hop_count,two_hop_count,multi_hop_count)
  VALUES (416,'search','en','fixture','fixture',1,0,0) RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	q := generatedQuestion{HopType: "one_hop", Question: "What is A?", Answer: "A is defined", ReasoningSteps: []string{"Read A"}, StepLineIDs: [][]int{{2}}, Evidence: []questionEvidence{{LineID: 2, Text: "A is defined"}}}
	if err := saveGeneratedQuestions(ctx, db, id, 416, []generatedQuestion{q}); err != nil {
		t.Fatal(err)
	}
	run, questions, err := loadQuestionRun(ctx, db, id)
	if err != nil || run.Status != "done" || len(questions) != 1 || questions[0].Evidence[0].Text != "A is defined" {
		t.Fatalf("saved run: %+v questions=%+v error=%v", run, questions, err)
	}
	runs, err := loadQuestionRuns(ctx, db, 416)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%v error=%v", runs, err)
	}
	var expiredID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO testbed.generated_question_runs
  (input_record_id,purpose,lang,model_ref,prompt_name,created_at)
  VALUES (416,'expired','en','fixture','fixture',NOW()-INTERVAL '11 minutes') RETURNING id`).Scan(&expiredID); err != nil {
		t.Fatal(err)
	}
	expired, _, err := loadQuestionRun(ctx, db, expiredID)
	if err != nil || expired.Status != "failed" {
		t.Fatalf("expiration: %+v %v", expired, err)
	}
	if err := saveGeneratedQuestions(ctx, db, expiredID, 416, []generatedQuestion{q}); err == nil {
		t.Fatal("expired run accepted late output")
	}
	_, late, err := loadQuestionRun(ctx, db, expiredID)
	if err != nil || len(late) != 0 {
		t.Fatalf("partial questions saved for expired run: %+v %v", late, err)
	}
}
