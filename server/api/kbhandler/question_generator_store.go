package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const questionRunColumns = `id, input_record_id, purpose, description,
 one_hop_count, two_hop_count, multi_hop_count, lang, status, error_msg,
 model_name, prompt_name, source_sha256, created_at, finished_at,
 total_one_hop_count, total_two_hop_count, total_multi_hop_count,
 generated_one_hop_count, generated_two_hop_count, generated_multi_hop_count, documents_processed, expires_at`

const generatedQuestionColumns = `id, run_id, input_record_id, hop_type, question, answer,
 reasoning_steps, step_line_ids, evidence, metric_id, metric_snapshot, set_no, lang, source_sha256, purpose,
 prompt_name, model_name, created_at`

func scanGeneratedQuestion(row interface{ Scan(...any) error }) (generatedQuestion, error) {
	var q generatedQuestion
	var steps, stepLines, evidence, metricSnapshot []byte
	if err := row.Scan(&q.ID, &q.RunID, &q.InputRecordID, &q.HopType, &q.Question, &q.Answer, &steps, &stepLines, &evidence, &q.MetricID, &metricSnapshot, &q.SetNo, &q.Lang, &q.SourceSHA256, &q.Purpose, &q.PromptName, &q.ModelName, &q.CreatedAt); err != nil {
		return q, err
	}
	for _, v := range []struct {
		raw    []byte
		target any
	}{{steps, &q.ReasoningSteps}, {stepLines, &q.StepLineIDs}, {evidence, &q.Evidence}} {
		if err := json.Unmarshal(v.raw, v.target); err != nil {
			return q, err
		}
	}
	q.MetricSnapshot = json.RawMessage(metricSnapshot)
	return q, nil
}

func scanQuestionRun(row interface{ Scan(...any) error }) (questionGenerationRun, error) {
	var run questionGenerationRun
	var inputID sql.NullInt64
	err := row.Scan(&run.ID, &inputID, &run.Purpose, &run.Description,
		&run.OneHopCount, &run.TwoHopCount, &run.MultiHopCount, &run.Lang, &run.Status,
		&run.ErrorMsg, &run.ModelName, &run.PromptName, &run.SourceSHA256, &run.CreatedAt, &run.FinishedAt,
		&run.TotalOneHopCount, &run.TotalTwoHopCount, &run.TotalMultiHopCount,
		&run.GeneratedOneHopCount, &run.GeneratedTwoHopCount, &run.GeneratedMultiHopCount, &run.DocumentsProcessed, &run.ExpiresAt)
	run.InputRecordID = inputID.Int64
	return run, err
}

func expireQuestionRuns(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `UPDATE testbed.generated_question_runs
 SET status = 'failed', error_msg = 'generation interrupted or timed out', finished_at = NOW()
 WHERE status = 'running' AND expires_at <= NOW()`)
	return err
}

func loadQuestionRuns(ctx context.Context, db *sql.DB, recordID int64) ([]questionGenerationRun, error) {
	if err := expireQuestionRuns(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+questionRunColumns+` FROM testbed.generated_question_runs
 WHERE $1::bigint = 0 OR input_record_id = $1 OR (input_record_id IS NULL AND EXISTS
 (SELECT 1 FROM testbed.generated_questions q WHERE q.run_id = generated_question_runs.id AND q.input_record_id = $1))
 ORDER BY id DESC LIMIT 50`, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []questionGenerationRun{}
	for rows.Next() {
		run, err := scanQuestionRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func loadQuestionRun(ctx context.Context, db *sql.DB, id int64) (questionGenerationRun, []generatedQuestion, error) {
	if err := expireQuestionRuns(ctx, db); err != nil {
		return questionGenerationRun{}, nil, err
	}
	run, err := scanQuestionRun(db.QueryRowContext(ctx, `SELECT `+questionRunColumns+` FROM testbed.generated_question_runs WHERE id = $1`, id))
	if err != nil {
		return run, nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+generatedQuestionColumns+` FROM testbed.generated_questions WHERE run_id = $1 ORDER BY id`, id)
	if err != nil {
		return run, nil, err
	}
	defer rows.Close()
	questions := []generatedQuestion{}
	for rows.Next() {
		q, err := scanGeneratedQuestion(rows)
		if err != nil {
			return run, nil, err
		}
		questions = append(questions, q)
	}
	return run, questions, rows.Err()
}

// Each document and its progress commit together. A late worker cannot change a
// terminal run; documents committed before a later failure remain available.
func saveGeneratedQuestions(ctx context.Context, db *sql.DB, runID, recordID int64, questions []generatedQuestion, done bool) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	oneHop, twoHop, multiHop := 0, 0, 0
	for _, q := range questions {
		switch q.HopType {
		case "one_hop":
			oneHop++
		case "two_hop":
			twoHop++
		case "multi_hop":
			multiHop++
		}
		steps, err := json.Marshal(q.ReasoningSteps)
		if err != nil {
			return err
		}
		stepLines, err := json.Marshal(q.StepLineIDs)
		if err != nil {
			return err
		}
		evidence, err := json.Marshal(q.Evidence)
		if err != nil {
			return err
		}
		metricSnapshot := q.MetricSnapshot
		if len(metricSnapshot) == 0 {
			metricSnapshot = json.RawMessage(`{}`)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO testbed.generated_questions
   (run_id, input_record_id, hop_type, question, answer, reasoning_steps, step_line_ids, evidence,
    metric_id, metric_snapshot, set_no, lang, source_sha256, purpose, prompt_name, model_name)
   SELECT $1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8::jsonb,$9,$10::jsonb,$11,$12,$13,
    r.purpose, r.prompt_name, r.model_name FROM testbed.generated_question_runs r WHERE r.id = $1`,
			runID, recordID, q.HopType, q.Question, q.Answer, string(steps), string(stepLines), string(evidence),
			q.MetricID, string(metricSnapshot), q.SetNo, q.Lang, q.SourceSHA256); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE testbed.generated_question_runs SET
 status = CASE WHEN $2 THEN 'done' ELSE 'running' END,
 finished_at = CASE WHEN $2 THEN NOW() ELSE finished_at END,
 generated_one_hop_count = generated_one_hop_count + $3,
 generated_two_hop_count = generated_two_hop_count + $4,
 generated_multi_hop_count = generated_multi_hop_count + $5,
 documents_processed = documents_processed + 1
 WHERE id = $1 AND status = 'running' AND expires_at > NOW()`, runID, done, oneHop, twoHop, multiHop)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("generation run expired or is already terminal")
	}
	return tx.Commit()
}
