package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
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
	if _, err := db.ExecContext(ctx, `CREATE SCHEMA kb; CREATE TABLE kb.inputs (id BIGINT PRIMARY KEY); INSERT INTO kb.inputs VALUES (416), (417), (418)`); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, filename := range []string{"20261024000001_create_generated_questions.sql", "20261024000011_question_generator_metric_batches.sql"} {
		raw, err := os.ReadFile(filepath.Join("../../../project_migrations", filename))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filename), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 20261024000001); err != nil {
		t.Fatalf("original migration up: %v", err)
	}
	var legacyID int64
	if err := db.QueryRowContext(ctx, `INSERT INTO testbed.generated_question_runs
  (input_record_id,purpose,lang,model_ref,prompt_name,one_hop_count,two_hop_count,multi_hop_count,status,source_sha256)
  VALUES (416,'search','zh-cn','fixture','fixture',1,0,0,'done','legacy-hash') RETURNING id`).Scan(&legacyID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO testbed.generated_questions
  (run_id,input_record_id,hop_type,question,answer,reasoning_steps,step_line_ids,evidence)
  VALUES ($1,416,'one_hop','legacy question','legacy answer','["Read A"]','[[2]]','[{"line_id":2,"text":"A is defined"}]')`, legacyID); err != nil {
		t.Fatal(err)
	}
	for _, action := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"new migration up", func(ctx context.Context) error { _, err := provider.Up(ctx); return err }},
		{"new migration down", func(ctx context.Context) error { _, err := provider.Down(ctx); return err }},
		{"new migration re-up", func(ctx context.Context) error { _, err := provider.Up(ctx); return err }},
	} {
		if err := action.run(ctx); err != nil {
			t.Fatalf("%s: %v", action.name, err)
		}
		var retained int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM testbed.generated_questions WHERE run_id=$1`, legacyID).Scan(&retained); err != nil || retained != 1 {
			t.Fatalf("%s lost legacy row: count=%d err=%v", action.name, retained, err)
		}
	}
	legacy, legacyQuestions, err := loadQuestionRun(ctx, db, legacyID)
	if err != nil || legacy.Lang != "zh-cn" || legacy.GeneratedOneHopCount != 1 || legacy.DocumentsProcessed != 1 || len(legacyQuestions) != 1 {
		t.Fatalf("legacy run: %+v questions=%+v error=%v", legacy, legacyQuestions, err)
	}
	if legacyQuestions[0].Lang != "zh-cn" || legacyQuestions[0].SourceSHA256 != "legacy-hash" || legacy.TotalOneHopCount != 1 || !legacy.ExpiresAt.Equal(legacy.CreatedAt.Add(10*time.Minute)) {
		t.Fatalf("legacy backfill: %+v %+v", legacy, legacyQuestions[0])
	}

	insertRun := func(recordID any, totalOne, totalTwo, totalMulti int, expires string) int64 {
		t.Helper()
		var id int64
		if err := db.QueryRowContext(ctx, `INSERT INTO testbed.generated_question_runs
  (input_record_id,purpose,lang,model_ref,prompt_name,one_hop_count,two_hop_count,multi_hop_count,
   total_one_hop_count,total_two_hop_count,total_multi_hop_count,expires_at)
  VALUES ($1,'search_metrics','mixed','fixture','fixture',1,1,1,$2,$3,$4,NOW()+$5::interval) RETURNING id`,
			recordID, totalOne, totalTwo, totalMulti, expires).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	question := func(hop, text, lang, metricID, hash string) generatedQuestion {
		return generatedQuestion{HopType: hop, Question: text, Answer: "A is defined", ReasoningSteps: []string{"Read A"},
			StepLineIDs: [][]int{{2}}, Evidence: []questionEvidence{{LineID: 2, Text: "A is defined"}},
			MetricID: metricID, MetricSnapshot: json.RawMessage(`{"name":"temperature","value":42}`), SetNo: 2, Lang: lang, SourceSHA256: hash}
	}
	batchID := insertRun(nil, 2, 1, 1, "1 hour")
	// Batch runs retain their hour deadline after the old ten-minute cutoff.
	if _, err := db.ExecContext(ctx, `UPDATE testbed.generated_question_runs SET created_at=NOW()-INTERVAL '11 minutes' WHERE id=$1`, batchID); err != nil {
		t.Fatal(err)
	}
	first := question("one_hop", "What is A?", "en", "metric-416", "hash-416")
	if err := saveGeneratedQuestions(ctx, db, batchID, 416, []generatedQuestion{first}, false); err != nil {
		t.Fatal(err)
	}
	run, questions, err := loadQuestionRun(ctx, db, batchID)
	if err != nil || run.Status != "running" || run.InputRecordID != 0 || run.DocumentsProcessed != 1 || run.GeneratedOneHopCount != 1 || run.GeneratedTwoHopCount != 0 || run.FinishedAt != nil || len(questions) != 1 {
		t.Fatalf("incremental progress: %+v questions=%+v error=%v", run, questions, err)
	}
	if questions[0].InputRecordID != 416 || questions[0].MetricID != "metric-416" || questions[0].Lang != "en" || questions[0].SourceSHA256 != "hash-416" || questions[0].SetNo != 2 || !strings.Contains(string(questions[0].MetricSnapshot), "temperature") {
		t.Fatalf("question provenance round trip: %+v", questions[0])
	}
	second := []generatedQuestion{
		question("one_hop", "什么是 B？", "zh-cn", "metric-417", "hash-417"),
		question("two_hop", "How do B and C relate?", "en", "metric-417", "hash-417"),
		question("multi_hop", "How do A, B and C relate?", "en", "metric-417", "hash-417"),
	}
	if err := saveGeneratedQuestions(ctx, db, batchID, 417, second, true); err != nil {
		t.Fatal(err)
	}
	run, questions, err = loadQuestionRun(ctx, db, batchID)
	if err != nil || run.Status != "done" || run.FinishedAt == nil || run.DocumentsProcessed != 2 || run.GeneratedOneHopCount != run.TotalOneHopCount || run.GeneratedTwoHopCount != run.TotalTwoHopCount || run.GeneratedMultiHopCount != run.TotalMultiHopCount || len(questions) != 4 {
		t.Fatalf("completed batch: %+v questions=%+v error=%v", run, questions, err)
	}
	if questions[1].Lang != "zh-cn" || questions[1].InputRecordID != 417 {
		t.Fatalf("per-question document/language: %+v", questions[1])
	}
	for _, tc := range []struct {
		recordID int64
		count    int
	}{{0, 2}, {416, 2}, {417, 1}, {418, 0}} {
		runs, err := loadQuestionRuns(ctx, db, tc.recordID)
		if err != nil || len(runs) != tc.count {
			t.Fatalf("runs for %d: got=%d want=%d error=%v", tc.recordID, len(runs), tc.count, err)
		}
	}
	if err := saveGeneratedQuestions(ctx, db, batchID, 418, []generatedQuestion{question("one_hop", "Late terminal?", "en", "m", "h")}, true); err == nil {
		t.Fatal("terminal run accepted late output")
	}
	_, questions, err = loadQuestionRun(ctx, db, batchID)
	if err != nil || len(questions) != 4 {
		t.Fatalf("terminal late output not rolled back: count=%d err=%v", len(questions), err)
	}

	singleID := insertRun(int64(418), 1, 0, 0, "10 minutes")
	if err := saveGeneratedQuestions(ctx, db, singleID, 418, []generatedQuestion{first}, true); err != nil {
		t.Fatal(err)
	}
	run, questions, err = loadQuestionRun(ctx, db, singleID)
	if err != nil || run.Status != "done" || run.InputRecordID != 418 || run.DocumentsProcessed != 1 || run.GeneratedOneHopCount != 1 || len(questions) != 1 || questions[0].InputRecordID != 418 {
		t.Fatalf("single-document save: %+v questions=%+v error=%v", run, questions, err)
	}

	partialID := insertRun(nil, 3, 0, 0, "1 hour")
	if err := saveGeneratedQuestions(ctx, db, partialID, 416, []generatedQuestion{first}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE testbed.generated_question_runs SET expires_at=NOW()-INTERVAL '1 second' WHERE id=$1`, partialID); err != nil {
		t.Fatal(err)
	}
	// Exercise the save guard itself before a read invokes the expirer.
	if err := saveGeneratedQuestions(ctx, db, partialID, 417, []generatedQuestion{second[0]}, true); err == nil {
		t.Fatal("expired batch accepted late output")
	}
	run, questions, err = loadQuestionRun(ctx, db, partialID)
	if err != nil || run.Status != "failed" || run.DocumentsProcessed != 1 || run.GeneratedOneHopCount != 1 || len(questions) != 1 || questions[0].InputRecordID != 416 {
		t.Fatalf("failed batch lost saved progress: %+v questions=%+v error=%v", run, questions, err)
	}
	if err := saveGeneratedQuestions(ctx, db, partialID, 417, []generatedQuestion{second[0]}, true); err == nil {
		t.Fatal("failed batch accepted late output")
	}

	// Down must refuse before mutations, leaving all batch data readable.
	if _, err := provider.Down(ctx); err == nil || !strings.Contains(err.Error(), "while batch runs exist") {
		t.Fatalf("unsafe batch downgrade was not refused: %v", err)
	}
	run, questions, err = loadQuestionRun(ctx, db, batchID)
	if err != nil || run.Status != "done" || len(questions) != 4 || questions[1].Lang != "zh-cn" {
		t.Fatalf("refused downgrade changed data: %+v questions=%+v error=%v", run, questions, err)
	}
}
