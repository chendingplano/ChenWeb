package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

const questionRunColumns = `id, input_record_id, purpose, description,
 one_hop_count, two_hop_count, multi_hop_count, lang, status, error_msg,
 model_name, prompt_name, source_sha256, created_at, finished_at`

func scanQuestionRun(row interface{ Scan(...any) error }) (questionGenerationRun, error) {
	var run questionGenerationRun
	err := row.Scan(&run.ID, &run.InputRecordID, &run.Purpose, &run.Description,
		&run.OneHopCount, &run.TwoHopCount, &run.MultiHopCount, &run.Lang, &run.Status,
		&run.ErrorMsg, &run.ModelName, &run.PromptName, &run.SourceSHA256, &run.CreatedAt, &run.FinishedAt)
	return run, err
}

func expireQuestionRuns(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `UPDATE testbed.generated_question_runs
 SET status = 'failed', error_msg = 'generation interrupted or timed out', finished_at = NOW()
 WHERE status = 'running' AND created_at <= NOW() - INTERVAL '10 minutes'`)
	return err
}

func loadQuestionRuns(ctx context.Context, db *sql.DB, recordID int64) ([]questionGenerationRun, error) {
	if err := expireQuestionRuns(ctx, db); err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, `SELECT `+questionRunColumns+` FROM testbed.generated_question_runs
 WHERE input_record_id = $1 ORDER BY id DESC LIMIT 50`, recordID)
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
	rows, err := db.QueryContext(ctx, `SELECT id, run_id, input_record_id, hop_type, question, answer,
 reasoning_steps, step_line_ids, evidence FROM testbed.generated_questions WHERE run_id = $1 ORDER BY id`, id)
	if err != nil {
		return run, nil, err
	}
	defer rows.Close()
	questions := []generatedQuestion{}
	for rows.Next() {
		var q generatedQuestion
		var steps, stepLines, evidence []byte
		if err := rows.Scan(&q.ID, &q.RunID, &q.InputRecordID, &q.HopType, &q.Question, &q.Answer, &steps, &stepLines, &evidence); err != nil {
			return run, nil, err
		}
		for _, v := range []struct {
			raw    []byte
			target any
		}{{steps, &q.ReasoningSteps}, {stepLines, &q.StepLineIDs}, {evidence, &q.Evidence}} {
			if err := json.Unmarshal(v.raw, v.target); err != nil {
				return run, nil, err
			}
		}
		questions = append(questions, q)
	}
	return run, questions, rows.Err()
}

// All questions and the terminal state commit together. A worker that loses the
// race to expiration cannot resurrect an interrupted run or save partial output.
func saveGeneratedQuestions(ctx context.Context, db *sql.DB, runID, recordID int64, questions []generatedQuestion) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range questions {
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
		if _, err := tx.ExecContext(ctx, `INSERT INTO testbed.generated_questions
   (run_id, input_record_id, hop_type, question, answer, reasoning_steps, step_line_ids, evidence)
   VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8::jsonb)`, runID, recordID, q.HopType, q.Question, q.Answer, string(steps), string(stepLines), string(evidence)); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE testbed.generated_question_runs SET status = 'done', finished_at = NOW()
 WHERE id = $1 AND status = 'running' AND created_at > NOW() - INTERVAL '10 minutes'`, runID)
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
