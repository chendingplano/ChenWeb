package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

const summaryScorePrompt = "prompt-score-extract-summaries-v1.md"

type summaryScoreRequest struct {
	RecordID    int64  `json:"record_id"`
	Model       string `json:"model"`
	Lang        string `json:"lang"`
	GoldVersion string `json:"gold_version,omitempty"`
	GoldModel   string `json:"gold_model,omitempty"`
	GoldRunID   string `json:"gold_run_id,omitempty"`
	Algorithm   string `json:"algorithm"`
}

type summaryScoreRun struct {
	ID            int64           `json:"id"`
	InputRecordID int64           `json:"input_record_id"`
	Title         string          `json:"title"`
	Algorithm     string          `json:"algorithm"`
	Lang          string          `json:"lang"`
	Status        string          `json:"status"`
	ModelName     string          `json:"model_name"`
	PromptName    string          `json:"prompt_name"`
	CreatedBy     string          `json:"created_by"`
	CreatedAt     time.Time       `json:"created_at"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
	ErrorMsg      string          `json:"error_msg,omitempty"`
	ErrorCode     string          `json:"error_code,omitempty"`
	Input         json.RawMessage `json:"input,omitempty"`
	Matches       json.RawMessage `json:"matches,omitempty"`
	Score         json.RawMessage `json:"score,omitempty"`
	Report        string          `json:"report,omitempty"`
}

type summaryScoreEvidence struct {
	Model     string
	Input     json.RawMessage
	Judgments json.RawMessage
	Score     json.RawMessage
	Report    string
}

func (r summaryScoreRequest) validate() error {
	if err := (metricScoreRequest{RecordID: r.RecordID, Model: r.Model, Lang: r.Lang, GoldVersion: r.GoldVersion, GoldModel: r.GoldModel, GoldRunID: r.GoldRunID}).validate(); err != nil {
		return err
	}
	if strings.TrimSpace(r.Algorithm) == "" {
		return fmt.Errorf("algorithm is required")
	}
	return nil
}

func ListSummaryScoreAlgorithms(c echo.Context) error {
	recordID, err := strconv.ParseInt(c.QueryParam("record_id"), 10, 64)
	if err != nil || recordID <= 0 {
		return metricScoreHTTPError(c, http.StatusBadRequest, "INVALID_REQUEST")
	}
	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `SELECT DISTINCT algorithm FROM kb.ex_summaries WHERE input_record_id=$1 AND summary_level=0 ORDER BY algorithm`, recordID)
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	defer rows.Close()
	algorithms := []string{}
	for rows.Next() {
		var value string
		if err = rows.Scan(&value); err != nil {
			return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
		}
		algorithms = append(algorithms, value)
	}
	if rows.Err() != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	return c.JSON(200, map[string]any{"status": true, "algorithms": algorithms})
}

func ListSummaryScoreGoldRuns(c echo.Context) error {
	recordID, err := strconv.ParseInt(c.QueryParam("record_id"), 10, 64)
	if err != nil || recordID <= 0 {
		return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
	}
	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `
SELECT r.skill_version,r.model_name,r.run_id,
 (SELECT count(*) FROM testbed.summaries s WHERE (s.skill_name,s.skill_version,s.model_name,s.run_id)=(r.skill_name,r.skill_version,r.model_name,r.run_id))
FROM testbed.summary_benchmark_runs r
WHERE r.input_record_id=$1 AND r.skill_name='generate-summaries-benchmark'
ORDER BY r.skill_version DESC,r.run_id DESC`, recordID)
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	defer rows.Close()
	result := []map[string]any{}
	for rows.Next() {
		var version, model, runID string
		var count int
		if err = rows.Scan(&version, &model, &runID, &count); err != nil {
			return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
		}
		result = append(result, map[string]any{"skill_version": version, "model_name": model, "benchmark_run_id": runID, "rows": count})
	}
	if rows.Err() != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	return c.JSON(200, map[string]any{"status": true, "runs": result})
}

func StartSummaryScore(c echo.Context) error {
	var req summaryScoreRequest
	if err := json.NewDecoder(io.LimitReader(c.Request().Body, 16384)).Decode(&req); err != nil {
		return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
	}
	req.Model, req.Algorithm = strings.TrimSpace(req.Model), strings.TrimSpace(req.Algorithm)
	if req.Lang == "" {
		req.Lang = "en"
	}
	if req.validate() != nil {
		return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
	}
	models, err := listMetricReviewModels()
	if err != nil {
		return metricScoreHTTPError(c, 503, "MODELS_UNAVAILABLE")
	}
	if !slices.Contains(models, req.Model) {
		return metricScoreHTTPError(c, 400, "UNKNOWN_MODEL")
	}
	if _, err = loadModelDefByRef(req.Model); err != nil {
		return metricScoreHTTPError(c, 503, "MODEL_UNAVAILABLE")
	}
	if _, err = scoreSkillScriptPath("SUMMARY_BENCHMARK_SCORER_PATH", "score-extract-summaries"); err != nil {
		return metricScoreHTTPError(c, 503, "SCORER_UNAVAILABLE")
	}
	db, ctx := ApiTypes.ProjectDBHandle, c.Request().Context()
	var title string
	if err = db.QueryRowContext(ctx, `SELECT COALESCE(title,'') FROM kb.inputs WHERE id=$1`, req.RecordID).Scan(&title); errors.Is(err, sql.ErrNoRows) {
		return metricScoreHTTPError(c, 404, "DOCUMENT_NOT_FOUND")
	} else if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	if _, err = db.ExecContext(ctx, `UPDATE kb.summary_score_runs SET status='failed',error_code='INTERRUPTED',error_msg='INTERRUPTED',finished_at=now() WHERE status='running' AND created_at < now()-interval '12 minutes'`); err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	requestJSON, _ := json.Marshal(req)
	var id int64
	err = db.QueryRowContext(ctx, `INSERT INTO kb.summary_score_runs(input_record_id,title,algorithm,lang,status,model_name,prompt_name,created_by,request)
VALUES($1,$2,$3,$4,'running',$5,$6,$7,$8)
ON CONFLICT (input_record_id) WHERE status='running' DO NOTHING RETURNING id`, req.RecordID, title, req.Algorithm, req.Lang, req.Model, summaryScorePrompt, c.Get("metric-score-user-name"), string(requestJSON)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var existing int64
		if err = db.QueryRowContext(ctx, `SELECT id FROM kb.summary_score_runs WHERE input_record_id=$1 AND status='running'`, req.RecordID).Scan(&existing); err != nil {
			return metricScoreHTTPError(c, 409, "RUN_CONFLICT")
		}
		run, err := loadSummaryScoreRun(ctx, db, existing, true)
		if err != nil {
			return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
		}
		return c.JSON(http.StatusAccepted, map[string]any{"status": true, "run": run})
	}
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	userID, _ := c.Get("metric-score-user-id").(string)
	go runSummaryScore(db, id, req, userID)
	run, err := loadSummaryScoreRun(ctx, db, id, true)
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	return c.JSON(http.StatusAccepted, map[string]any{"status": true, "run": run})
}

const summaryScoreColumns = `id,input_record_id,title,algorithm,lang,status,model_name,prompt_name,created_by,created_at,finished_at,error_msg,error_code,input_snapshot,judgments_snapshot,score_snapshot,report`

func scanSummaryScoreRun(row metricScoreScanner) (*summaryScoreRun, error) {
	var r summaryScoreRun
	var input, judgments, score []byte
	err := row.Scan(&r.ID, &r.InputRecordID, &r.Title, &r.Algorithm, &r.Lang, &r.Status, &r.ModelName, &r.PromptName, &r.CreatedBy, &r.CreatedAt, &r.FinishedAt, &r.ErrorMsg, &r.ErrorCode, &input, &judgments, &score, &r.Report)
	r.Input, r.Matches, r.Score = input, judgments, score
	return &r, err
}

func loadSummaryScoreRun(ctx context.Context, db *sql.DB, id int64, detail bool) (*summaryScoreRun, error) {
	r, err := scanSummaryScoreRun(db.QueryRowContext(ctx, `SELECT `+summaryScoreColumns+` FROM kb.summary_score_runs WHERE id=$1`, id))
	if r != nil && !detail {
		r.Input, r.Matches, r.Report = nil, nil, ""
	}
	return r, err
}

func ListSummaryScores(c echo.Context) error {
	recordID, limit, offset := int64(0), 20, 0
	var err error
	if value := c.QueryParam("record_id"); value != "" {
		if recordID, err = strconv.ParseInt(value, 10, 64); err != nil || recordID <= 0 {
			return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
		}
	}
	if value := c.QueryParam("limit"); value != "" {
		if limit, err = strconv.Atoi(value); err != nil || limit < 1 || limit > 100 {
			return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
		}
	}
	if value := c.QueryParam("offset"); value != "" {
		if offset, err = strconv.Atoi(value); err != nil || offset < 0 {
			return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
		}
	}
	db, ctx := ApiTypes.ProjectDBHandle, c.Request().Context()
	if _, err = db.ExecContext(ctx, `UPDATE kb.summary_score_runs SET status='failed',error_code='INTERRUPTED',error_msg='INTERRUPTED',finished_at=now() WHERE status='running' AND created_at < now()-interval '12 minutes'`); err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	var total int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM kb.summary_score_runs WHERE ($1::bigint=0 OR input_record_id=$1)`, recordID).Scan(&total); err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	rows, err := db.QueryContext(ctx, `SELECT id,input_record_id,title,algorithm,lang,status,model_name,prompt_name,created_by,created_at,finished_at,error_msg,error_code,NULL,NULL,score_snapshot,'' FROM kb.summary_score_runs WHERE ($1::bigint=0 OR input_record_id=$1) ORDER BY id DESC LIMIT $2 OFFSET $3`, recordID, limit, offset)
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	defer rows.Close()
	result := []*summaryScoreRun{}
	for rows.Next() {
		r, scanErr := scanSummaryScoreRun(rows)
		if scanErr != nil {
			return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
		}
		result = append(result, r)
	}
	if rows.Err() != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	return c.JSON(200, map[string]any{"status": true, "runs": result, "total": total})
}

func GetSummaryScore(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
	}
	run, err := loadSummaryScoreRun(c.Request().Context(), ApiTypes.ProjectDBHandle, id, true)
	if errors.Is(err, sql.ErrNoRows) {
		return metricScoreHTTPError(c, 404, "RUN_NOT_FOUND")
	}
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	return c.JSON(200, map[string]any{"status": true, "run": run})
}

func DownloadSummaryScoreArtifact(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
	}
	run, err := loadSummaryScoreRun(c.Request().Context(), ApiTypes.ProjectDBHandle, id, true)
	if errors.Is(err, sql.ErrNoRows) {
		return metricScoreHTTPError(c, 404, "RUN_NOT_FOUND")
	}
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	kind := c.Param("kind")
	var body []byte
	contentType, filename := "application/json", kind+".json"
	switch kind {
	case "input":
		body = run.Input
	case "judgments":
		body = run.Matches
		filename = "judgments.json"
	case "score":
		body = run.Score
	case "report":
		body, contentType, filename = []byte(run.Report), "text/markdown; charset=utf-8", "report.md"
	default:
		return metricScoreHTTPError(c, 404, "ARTIFACT_NOT_FOUND")
	}
	if len(body) == 0 {
		return metricScoreHTTPError(c, 404, "ARTIFACT_NOT_FOUND")
	}
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="summary-benchmark-%d-%s"`, id, filename))
	return c.Blob(200, contentType, body)
}

func runSummaryScore(db *sql.DB, id int64, req summaryScoreRequest, userID string) {
	logger := loggerutil.CreateDefaultLogger("summary-score-" + strconv.FormatInt(id, 10))
	logger.Info("summary benchmark started", "run_id", id, "record_id", req.RecordID, "algorithm", req.Algorithm, "model", req.Model)
	ctx, cancel := context.WithTimeout(context.Background(), metricReviewRunTimeout)
	defer cancel()
	evidence, err := executeSummaryScore(ctx, db, logger, id, req, userID)
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer finishCancel()
	status, code := "done", ""
	if err != nil {
		status, code = "failed", metricScoreErrorCode(err)
		logger.Error("summary benchmark failed", "run_id", id, "error_code", code)
	}
	_, storeErr := db.ExecContext(finishCtx, `UPDATE kb.summary_score_runs SET status=$2,model_name=$3,input_snapshot=$4,judgments_snapshot=$5,score_snapshot=$6,report=$7,error_msg=$8,error_code=$8,finished_at=now() WHERE id=$1 AND status='running'`, id, status, evidence.Model, nullableMetricScoreJSON(evidence.Input), nullableMetricScoreJSON(evidence.Judgments), nullableMetricScoreJSON(evidence.Score), evidence.Report, code)
	if storeErr != nil {
		logger.Error("store summary benchmark failed", "run_id", id, "err", storeErr)
	}
}

func executeSummaryScore(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, id int64, req summaryScoreRequest, userID string) (e summaryScoreEvidence, err error) {
	e.Model = req.Model
	script, err := scoreSkillScriptPath("SUMMARY_BENCHMARK_SCORER_PATH", "score-extract-summaries")
	if err != nil {
		return e, err
	}
	dataHome := strings.TrimSpace(os.Getenv("DATA_HOME_DIR"))
	if dataHome == "" {
		return e, metricScoreFailure{"DATA_HOME_UNAVAILABLE"}
	}
	var poolDatabase, poolUser string
	if err = db.QueryRowContext(ctx, `SELECT current_database(),current_user`).Scan(&poolDatabase, &poolUser); err != nil {
		return e, err
	}
	poolConfig := metricScoreDatabaseConfig()
	if poolDatabase != poolConfig.ProjectDBName || poolUser != poolConfig.UserName {
		return e, metricScoreFailure{"DATABASE_MISMATCH"}
	}
	root := filepath.Join(dataHome, "summary-score-runs", strconv.FormatInt(id, 10))
	if err = os.MkdirAll(root, 0700); err != nil {
		return e, err
	}
	frozen, hashes, err := freezeScoreTools(root, script, "score-extract-summaries", "generate-summaries-benchmark")
	if err != nil {
		return e, err
	}
	dir := filepath.Join(root, "evidence")
	args := []string{"export", "--record-id", strconv.FormatInt(req.RecordID, 10), "--candidate", "extract", "--algorithm", req.Algorithm, "--out-dir", dir}
	if req.GoldRunID != "" {
		args = append(args, "--gold-version", req.GoldVersion, "--gold-model", req.GoldModel, "--gold-run-id", req.GoldRunID)
	}
	if err = runMetricScoreScriptAt(ctx, frozen, args...); err != nil {
		return e, err
	}
	rawInput, err := os.ReadFile(filepath.Join(dir, "input.json"))
	if err != nil {
		return e, err
	}
	var input map[string]any
	if err = json.Unmarshal(rawInput, &input); err != nil {
		return e, err
	}
	prompt, err := os.ReadFile(filepath.Join("prompts", summaryScorePrompt))
	if err != nil {
		return e, metricScoreFailure{"PROMPT_UNAVAILABLE"}
	}
	hashes[summaryScorePrompt] = metricScoreDigest(prompt)
	if err = os.WriteFile(filepath.Join(root, summaryScorePrompt), prompt, 0600); err != nil {
		return e, err
	}
	cfg, client, err := newMetricReviewLLMClient(logger, req.Model)
	if err != nil {
		return e, metricScoreFailure{"MODEL_UNAVAILABLE"}
	}
	e.Model = cfg.ModelName
	input["provenance"] = map[string]any{"implementation_sha256": hashes, "prompt_name": summaryScorePrompt, "model_profile": req.Model, "scorer_model": cfg.ModelName, "lang": req.Lang}
	e.Input, err = json.MarshalIndent(input, "", "  ")
	if err != nil {
		return e, err
	}
	if err = os.WriteFile(filepath.Join(dir, "input.json"), e.Input, 0600); err != nil {
		return e, err
	}
	if utf8RuneCount(e.Input) > metricReviewMaxInputChars {
		return e, metricScoreFailure{"INPUT_TOO_LARGE"}
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("MODEL_DEFAULT_REASONING_POLICY")), "no-reasoning") {
		client.ThinkingType = "disabled"
	}
	inputText := "output_language: " + req.Lang + "\nSCORING_INPUT\n" + string(e.Input)
	for attempt := 1; attempt <= 2; attempt++ {
		logger.Info("calling LLM for summary judgments", "run_id", id, "record_id", req.RecordID, "model", cfg.ModelName, "attempt", attempt)
		payload, callErr := client.ExtractJSON(ctx, llmclients.JSONExtractionInput{UserID: userID, PromptName: summaryScorePrompt, PromptText: string(prompt), ModelName: cfg.ModelName, InputText: inputText, RecordID: req.RecordID, RunID: id, CallReason: "score_extract_summaries", CallLoc: "MID-20261021-01"})
		if callErr != nil {
			if ctx.Err() != nil {
				return e, ctx.Err()
			}
			return e, metricScoreFailure{"LLM_FAILED"}
		}
		chunks, ok := payload["chunks"]
		if !ok {
			return e, metricScoreFailure{"INVALID_JUDGMENTS"}
		}
		limitations := []any{}
		if rawLimitations, present := payload["limitations"]; present {
			var ok bool
			limitations, ok = rawLimitations.([]any)
			if !ok {
				return e, metricScoreFailure{"INVALID_JUDGMENTS"}
			}
		}
		limitations = append(limitations, "The selected scoring model performed the semantic judgments; no independent human reviewer was involved.")
		fingerprint, fingerprintErr := summaryInputFingerprint(ctx, frozen, e.Input)
		if fingerprintErr != nil {
			return e, fingerprintErr
		}
		judgments := map[string]any{"input_sha256": fingerprint, "reviewer": "model:" + cfg.ModelName, "limitations": limitations, "chunks": chunks}
		e.Judgments, err = json.MarshalIndent(judgments, "", "  ")
		if err != nil {
			return e, err
		}
		if err = os.WriteFile(filepath.Join(dir, "judgments.json"), e.Judgments, 0600); err != nil {
			return e, err
		}
		if scoreErr := runMetricScoreScriptAt(ctx, frozen, "score", "--dir", dir, "--scorer-model", cfg.ModelName); scoreErr != nil {
			var validation metricScoreValidationError
			if !errors.As(scoreErr, &validation) {
				return e, scoreErr
			}
			if attempt == 2 {
				return e, scoreErr
			}
			feedback, _ := json.Marshal(map[string]any{"previous_judgments": judgments, "validation_error": validation.Reason})
			inputText += "\nJUDGMENT_REPAIR\n" + string(feedback)
			continue
		}
		break
	}
	e.Score, err = os.ReadFile(filepath.Join(dir, "score.json"))
	if err != nil {
		return e, err
	}
	report, err := os.ReadFile(filepath.Join(dir, "report.md"))
	if err != nil {
		return e, err
	}
	e.Report = string(report)
	return e, nil
}

func summaryInputFingerprint(ctx context.Context, script string, input []byte) (string, error) {
	cmd := exec.CommandContext(ctx, "python3", "-c", "import json,score_io,sys; print(score_io.fingerprint(json.load(sys.stdin)))")
	cmd.Dir = filepath.Dir(script)
	cmd.Stdin = strings.NewReader(string(input))
	output, err := cmd.Output()
	if err != nil {
		return "", metricScoreFailure{"SCORER_UNAVAILABLE"}
	}
	return strings.TrimSpace(string(output)), nil
}

func utf8RuneCount(value []byte) int { return len([]rune(string(value))) }
