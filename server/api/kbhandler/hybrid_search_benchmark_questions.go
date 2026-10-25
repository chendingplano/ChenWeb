package kbhandler

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

// The "Tests to run" dialog searches the questions of one purpose and lets the user pick which ones
// a run uses. It returns every match (up to hybridBenchPickerLimit) so "select all" covers the
// whole search, not one page.

const (
	hybridBenchPickerLimit   = 2000
	hybridBenchMaxSelected   = 5000
	hybridBenchMaxWordsField = 200
)

var hybridBenchHopTypes = []string{"one_hop", "two_hop", "multi_hop"}

type hybridBenchQuestionFilters struct {
	questionBrowserFilters
	HopType, MetricID          string
	QuestionWords, AnswerWords []string
}

func parseHybridBenchQuestionFilters(c echo.Context) (hybridBenchQuestionFilters, error) {
	base, err := parseQuestionBrowserFilters(c)
	f := hybridBenchQuestionFilters{questionBrowserFilters: base}
	if err != nil {
		return f, err
	}
	if !slices.Contains(hybridBenchPurposes, f.Purpose) {
		return f, fmt.Errorf("unsupported purpose")
	}
	f.HopType = strings.TrimSpace(c.QueryParam("hop_type"))
	if f.HopType != "" && !slices.Contains(hybridBenchHopTypes, f.HopType) {
		return f, fmt.Errorf("invalid hop_type")
	}
	f.MetricID = strings.TrimSpace(c.QueryParam("metric_id"))
	for _, field := range []struct {
		name   string
		target *[]string
	}{{"question", &f.QuestionWords}, {"answer", &f.AnswerWords}} {
		raw := c.QueryParam(field.name)
		if len([]rune(raw)) > hybridBenchMaxWordsField {
			return f, fmt.Errorf("%s is too long", field.name)
		}
		*field.target = strings.Fields(raw)
	}
	return f, nil
}

// where extends the question browser's filters. Every word must occur in the field (case-insensitive
// substring, which also works for Chinese text without spaces); the metric ID matches exactly.
func (f hybridBenchQuestionFilters) where() (string, []any) {
	where, args := f.questionBrowserFilters.where()
	clauses := []string{}
	add := func(clause string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(clause, len(args)))
	}
	if f.HopType != "" {
		add("hop_type = $%d", f.HopType)
	}
	if f.MetricID != "" {
		add("metric_id = $%d", f.MetricID)
	}
	for _, w := range f.QuestionWords {
		add(`question ILIKE '%%' || $%d || '%%'`, escapeLike(w))
	}
	for _, w := range f.AnswerWords {
		add(`answer ILIKE '%%' || $%d || '%%'`, escapeLike(w))
	}
	if len(clauses) == 0 {
		return where, args
	}
	if where == "" {
		return " WHERE " + strings.Join(clauses, " AND "), args
	}
	return where + " AND " + strings.Join(clauses, " AND "), args
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

type hybridBenchPickerQuestion struct {
	ID            int64     `json:"id"`
	RunID         int64     `json:"run_id"`
	InputRecordID int64     `json:"input_record_id"`
	HopType       string    `json:"hop_type"`
	Lang          string    `json:"lang"`
	MetricID      string    `json:"metric_id"`
	Question      string    `json:"question"`
	Answer        string    `json:"answer"`
	CreatedAt     time.Time `json:"created_at"`
}

// ListHybridSearchBenchmarkQuestions returns the questions matching the dialog's filters, oldest first.
func ListHybridSearchBenchmarkQuestions(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("20261025-747")
	f, err := parseHybridBenchQuestionFilters(c)
	if err != nil {
		return questionGenerationError(c, http.StatusBadRequest, err.Error())
	}
	db, ctx := ApiTypes.ProjectDBHandle, c.Request().Context()
	where, args := f.where()
	var total int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM testbed.generated_questions`+where, args...).Scan(&total); err != nil {
		logger.Error("count benchmark questions failed", "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load questions")
	}
	args = append(args, hybridBenchPickerLimit)
	rows, err := db.QueryContext(ctx, `SELECT id, run_id, input_record_id, hop_type, lang, metric_id, question, answer, created_at
 FROM testbed.generated_questions`+where+fmt.Sprintf(" ORDER BY id LIMIT $%d", len(args)), args...)
	if err != nil {
		logger.Error("list benchmark questions failed", "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load questions")
	}
	defer rows.Close()
	questions := []hybridBenchPickerQuestion{}
	for rows.Next() {
		var q hybridBenchPickerQuestion
		if err := rows.Scan(&q.ID, &q.RunID, &q.InputRecordID, &q.HopType, &q.Lang, &q.MetricID, &q.Question, &q.Answer, &q.CreatedAt); err != nil {
			logger.Error("scan benchmark question failed", "error", err)
			return questionGenerationError(c, http.StatusInternalServerError, "failed to load questions")
		}
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		logger.Error("list benchmark questions failed", "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load questions")
	}
	return c.JSON(http.StatusOK, map[string]any{"questions": questions, "total": total, "limit": hybridBenchPickerLimit})
}

// loadHybridBenchQuestions returns the run's questions: the selected ones, or every question of the
// purpose when none are selected. A selected ID outside the purpose is an error.
func loadHybridBenchQuestions(ctx context.Context, db *sql.DB, purpose string, ids []int64) ([]hybridBenchQuestion, error) {
	query := `SELECT id, input_record_id, hop_type, lang, question, metric_id
 FROM testbed.generated_questions WHERE purpose = $1 AND metric_id <> ''`
	args := []any{purpose}
	if len(ids) > 0 {
		query += ` AND id = ANY($2)`
		args = append(args, ids)
	}
	rows, err := db.QueryContext(ctx, query+` ORDER BY id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var questions []hybridBenchQuestion
	for rows.Next() {
		var q hybridBenchQuestion
		if err := rows.Scan(&q.ID, &q.InputRecordID, &q.HopType, &q.Lang, &q.Question, &q.TargetArtifactID); err != nil {
			return nil, err
		}
		questions = append(questions, q)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) > 0 && len(questions) != len(ids) {
		return nil, errHybridBenchSelection
	}
	return questions, nil
}
