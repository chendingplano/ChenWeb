package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

// Dependencies are replaced in worker/handler tests; production uses the existing
// canonical file loader and configured LLM provider client, including usage logs.
var (
	loadQuestionGenerationLines = docprocessing.LoadRecordLinesByID
	newQuestionGenerationClient = func(modelRef string, logger ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, metricJSONExtractor, error) {
		cfg, client, err := newMetricReviewLLMClient(logger, modelRef)
		return cfg, client, err
	}
	launchQuestionGeneration = func(db *sql.DB, runID int64, req questionGenerationRequest, modelRef, userID string) {
		go runQuestionGeneration(db, runID, req, modelRef, userID)
	}
)

func questionGenerationError(c echo.Context, status int, message string) error {
	return c.JSON(status, errorResponse{Status: false, ErrorMsg: message})
}

// QuestionGeneratorAdmin protects writes and saved benchmark data, including
// direct API access outside the System Admin navigation.
func QuestionGeneratorAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		rc := EchoFactory.NewFromEcho(c, "20261024-681")
		defer rc.Close()
		user := rc.IsAuthenticated()
		if user == nil {
			return questionGenerationError(c, http.StatusUnauthorized, "UNAUTHORIZED")
		}
		if !metricScoreIsAdmin(user) {
			return questionGenerationError(c, http.StatusForbidden, "FORBIDDEN")
		}
		if ApiTypes.ProjectDBHandle == nil {
			return questionGenerationError(c, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE")
		}
		c.Set("question-generator-user-id", user.UserId)
		return next(c)
	}
}

func StartQuestionGeneration(c echo.Context) error {
	logger := loggerutil.CreateDefaultLogger("20261024-682")
	var req questionGenerationRequest
	decoder := json.NewDecoder(http.MaxBytesReader(c.Response(), c.Request().Body, 64*1024))
	if err := decoder.Decode(&req); err != nil {
		return questionGenerationError(c, http.StatusBadRequest, "invalid request body")
	}
	if err := req.normalize(); err != nil {
		return questionGenerationError(c, http.StatusBadRequest, err.Error())
	}
	db, ctx := ApiTypes.ProjectDBHandle, c.Request().Context()
	var exists bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM kb.inputs WHERE id = $1)`, req.InputRecordID).Scan(&exists); err != nil {
		logger.Error("question generation record lookup failed", "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load input record")
	}
	if !exists {
		return questionGenerationError(c, http.StatusNotFound, "input record not found")
	}
	modelRef := envOrDefault("QUESTION_GEN_MODEL_NAME", "deepseek-flash-processor")
	userID, _ := c.Get("question-generator-user-id").(string)
	run, err := scanQuestionRun(db.QueryRowContext(ctx, `INSERT INTO testbed.generated_question_runs
  (input_record_id,purpose,description,one_hop_count,two_hop_count,multi_hop_count,lang,model_ref,prompt_name,created_by)
  VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING `+questionRunColumns,
		req.InputRecordID, req.Purpose, req.Description, *req.OneHopCount, *req.TwoHopCount, *req.MultiHopCount, req.Lang, modelRef, questionGenerationPrompt, userID))
	if err != nil {
		logger.Error("question generation run insert failed", "record_id", req.InputRecordID, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to create generation run")
	}
	logger.Info("question generation started", "run_id", run.ID, "record_id", req.InputRecordID, "model_ref", modelRef, "user_id", userID)
	launchQuestionGeneration(db, run.ID, req, modelRef, userID)
	return c.JSON(http.StatusAccepted, map[string]any{"run": run})
}

func ListQuestionGenerationRuns(c echo.Context) error {
	id, err := strconv.ParseInt(c.QueryParam("input_record_id"), 10, 64)
	if err != nil || id <= 0 {
		return questionGenerationError(c, http.StatusBadRequest, "invalid input_record_id")
	}
	runs, err := loadQuestionRuns(c.Request().Context(), ApiTypes.ProjectDBHandle, id)
	if err != nil {
		loggerutil.CreateDefaultLogger("20261024-683").Error("list question generation runs failed", "record_id", id, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load generation runs")
	}
	return c.JSON(http.StatusOK, map[string]any{"runs": runs})
}

func GetQuestionGenerationRun(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return questionGenerationError(c, http.StatusBadRequest, "invalid run id")
	}
	run, questions, err := loadQuestionRun(c.Request().Context(), ApiTypes.ProjectDBHandle, id)
	if errors.Is(err, sql.ErrNoRows) {
		return questionGenerationError(c, http.StatusNotFound, "generation run not found")
	}
	if err != nil {
		loggerutil.CreateDefaultLogger("20261024-684").Error("load question generation run failed", "run_id", id, "error", err)
		return questionGenerationError(c, http.StatusInternalServerError, "failed to load generation run")
	}
	return c.JSON(http.StatusOK, map[string]any{"run": run, "questions": questions})
}

func runQuestionGeneration(db *sql.DB, id int64, req questionGenerationRequest, modelRef, userID string) {
	logger := loggerutil.CreateDefaultLogger("20261024-685")
	ctx, cancel := context.WithTimeout(context.Background(), questionGenerationTimeout)
	defer cancel()
	err := generateQuestionsForRun(ctx, db, id, req, modelRef, userID, logger)
	if err == nil {
		logger.Info("question generation completed", "run_id", id, "record_id", req.InputRecordID)
		return
	}
	logger.Error("question generation failed", "run_id", id, "record_id", req.InputRecordID, "error", err)
	// The model deadline may already have elapsed. Use a fresh bounded context to
	// persist failure, without ever overwriting a terminal run.
	cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if _, saveErr := db.ExecContext(cleanup, `UPDATE testbed.generated_question_runs
  SET status = 'failed', error_msg = $2, finished_at = NOW() WHERE id = $1 AND status = 'running'`, id, err.Error()); saveErr != nil {
		logger.Error("save question generation failure failed", "run_id", id, "error", saveErr)
	}
}

func generateQuestionsForRun(ctx context.Context, db *sql.DB, id int64, req questionGenerationRequest, modelRef, userID string, logger ApiTypes.JimoLogger) error {
	lines, err := loadQuestionGenerationLines(ctx, db, req.InputRecordID)
	if err != nil {
		logger.Error("load question source failed", "run_id", id, "error", err)
		return fmt.Errorf("cannot read this record's canonical line file")
	}
	sourceText, source, hash, err := buildQuestionGenerationSource(lines)
	if err != nil {
		return err
	}
	cfg, client, err := newQuestionGenerationClient(modelRef, logger)
	if err != nil {
		logger.Error("configure question model failed", "run_id", id, "error", err)
		return fmt.Errorf("cannot configure question generation model %q", modelRef)
	}
	prompt, _, _, err := docprocessing.LoadPromptByRef(questionGenerationPrompt)
	if err != nil {
		logger.Error("load question prompt failed", "run_id", id, "error", err)
		return fmt.Errorf("cannot load question generation prompt")
	}
	if _, err := db.ExecContext(ctx, `UPDATE testbed.generated_question_runs SET source_sha256 = $2, model_name = $3 WHERE id = $1 AND status = 'running'`, id, hash, cfg.ModelName); err != nil {
		return fmt.Errorf("cannot save source provenance")
	}
	input, err := json.Marshal(map[string]any{
		"input_record_id": req.InputRecordID, "purpose": req.Purpose, "description": req.Description, "lang": req.Lang,
		"counts":       map[string]int{"one_hop": *req.OneHopCount, "two_hop": *req.TwoHopCount, "multi_hop": *req.MultiHopCount},
		"source_lines": json.RawMessage(sourceText),
	})
	if err != nil {
		return fmt.Errorf("cannot format question generation input")
	}
	payload, err := client.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		UserID: userID, PromptName: questionGenerationPrompt, PromptText: prompt, ModelName: cfg.ModelName,
		InputText: string(input), CallReason: "question_generation", CallLoc: "20261024-685",
	})
	if err != nil {
		logger.Error("question generation model call failed", "run_id", id, "error", err)
		return fmt.Errorf("question generation model request failed or timed out")
	}
	questions, err := validateGeneratedQuestions(payload, req, source)
	if err != nil {
		return err
	}
	if err := saveGeneratedQuestions(ctx, db, id, req.InputRecordID, questions); err != nil {
		logger.Error("save generated questions failed", "run_id", id, "error", err)
		return fmt.Errorf("cannot save questions: run expired or database write failed")
	}
	return nil
}
