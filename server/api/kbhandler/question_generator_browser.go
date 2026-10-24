package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

type questionBrowserFilters struct {
	Purpose, Lang   string
	RunID, RecordID int64
	From, To        *time.Time
	Page, PageSize  int
}

func parseQuestionBrowserFilters(c echo.Context) (questionBrowserFilters, error) {
	f := questionBrowserFilters{Purpose: strings.TrimSpace(c.QueryParam("purpose")), Lang: strings.TrimSpace(c.QueryParam("lang")), Page: 1, PageSize: 50}
	for _, field := range []struct {
		name   string
		target *int64
	}{{"run_id", &f.RunID}, {"input_record_id", &f.RecordID}} {
		if raw := c.QueryParam(field.name); raw != "" {
			v, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || v < 1 {
				return f, fmt.Errorf("invalid %s", field.name)
			}
			*field.target = v
		}
	}
	for _, field := range []struct {
		name   string
		target *int
		max    int
	}{{"page", &f.Page, 1000000}, {"page_size", &f.PageSize, 100}} {
		if raw := c.QueryParam(field.name); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v < 1 || v > field.max {
				return f, fmt.Errorf("invalid %s", field.name)
			}
			*field.target = v
		}
	}
	for _, field := range []struct {
		name   string
		target **time.Time
	}{{"created_from", &f.From}, {"created_to", &f.To}} {
		if raw := c.QueryParam(field.name); raw != "" {
			v, err := time.Parse(time.RFC3339Nano, raw)
			if err != nil {
				return f, fmt.Errorf("invalid %s: use RFC3339 with timezone", field.name)
			}
			*field.target = &v
		}
	}
	if f.From != nil && f.To != nil && f.From.After(*f.To) {
		return f, fmt.Errorf("created_from must not be after created_to")
	}
	return f, nil
}

func (f questionBrowserFilters) where() (string, []any) {
	clauses := []string{}
	args := []any{}
	add := func(column, operator string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s %s $%d", column, operator, len(args)))
	}
	if f.Purpose != "" {
		add("purpose", "=", f.Purpose)
	}
	if f.RunID > 0 {
		add("run_id", "=", f.RunID)
	}
	if f.RecordID > 0 {
		add("input_record_id", "=", f.RecordID)
	}
	if f.Lang != "" {
		add("lang", "=", f.Lang)
	}
	if f.From != nil {
		add("created_at", ">=", *f.From)
	}
	if f.To != nil {
		add("created_at", "<=", *f.To)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

type questionBrowserResult struct {
	Questions []generatedQuestion `json:"questions"`
	Total     int64               `json:"total"`
	Page      int                 `json:"page"`
	PageSize  int                 `json:"page_size"`
}

func loadGeneratedQuestions(ctx context.Context, db *sql.DB, f questionBrowserFilters) (questionBrowserResult, error) {
	result := questionBrowserResult{Questions: []generatedQuestion{}, Page: f.Page, PageSize: f.PageSize}
	where, args := f.where()
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM testbed.generated_questions`+where, args...).Scan(&result.Total); err != nil {
		return result, err
	}
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := db.QueryContext(ctx, `SELECT `+generatedQuestionColumns+` FROM testbed.generated_questions`+where+fmt.Sprintf(" ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		q, err := scanGeneratedQuestion(rows)
		if err != nil {
			return result, err
		}
		result.Questions = append(result.Questions, q)
	}
	return result, rows.Err()
}

type questionBrowserOptions struct {
	Purposes  []string `json:"purposes"`
	RunIDs    []int64  `json:"run_ids"`
	RecordIDs []int64  `json:"record_ids"`
	Languages []string `json:"languages"`
}

func loadQuestionBrowserOptions(ctx context.Context, db *sql.DB) (questionBrowserOptions, error) {
	var options questionBrowserOptions
	var purposes, runs, records, languages []byte
	err := db.QueryRowContext(ctx, `SELECT
 COALESCE(jsonb_agg(DISTINCT purpose ORDER BY purpose),'[]'::jsonb),
 COALESCE(jsonb_agg(DISTINCT run_id ORDER BY run_id),'[]'::jsonb),
 COALESCE(jsonb_agg(DISTINCT input_record_id ORDER BY input_record_id),'[]'::jsonb),
 COALESCE(jsonb_agg(DISTINCT lang ORDER BY lang),'[]'::jsonb)
 FROM testbed.generated_questions`).Scan(&purposes, &runs, &records, &languages)
	if err != nil {
		return options, err
	}
	for _, field := range []struct {
		raw    []byte
		target any
	}{{purposes, &options.Purposes}, {runs, &options.RunIDs}, {records, &options.RecordIDs}, {languages, &options.Languages}} {
		if err := json.Unmarshal(field.raw, field.target); err != nil {
			return options, err
		}
	}
	return options, nil
}
func ListGeneratedQuestions(c echo.Context) error {
	f, err := parseQuestionBrowserFilters(c)
	if err != nil {
		return questionGenerationError(c, http.StatusBadRequest, err.Error())
	}
	result, err := loadGeneratedQuestions(c.Request().Context(), ApiTypes.ProjectDBHandle, f)
	if err != nil {
		loggerutil.CreateDefaultLogger("20261024-391").Error("query generated questions failed", "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load generated questions")
	}
	return c.JSON(http.StatusOK, result)
}
func ListQuestionBrowserFilters(c echo.Context) error {
	result, err := loadQuestionBrowserOptions(c.Request().Context(), ApiTypes.ProjectDBHandle)
	if err != nil {
		loggerutil.CreateDefaultLogger("20261024-392").Error("load question filters failed", "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load question filters")
	}
	return c.JSON(http.StatusOK, result)
}
