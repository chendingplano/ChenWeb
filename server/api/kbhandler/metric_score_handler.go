package kbhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

const metricScorePrompt = "prompt-score-extract-metrics-v1.md"

// A run stores immutable scoring evidence, independent of later extraction changes.
type metricScoreRun struct {
	ID            int64           `json:"id"`
	InputRecordID int64           `json:"input_record_id"`
	Title         string          `json:"title"`
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

type metricScoreRequest struct {
	RecordID    int64  `json:"record_id"`
	Model       string `json:"model"`
	Lang        string `json:"lang"`
	GoldVersion string `json:"gold_version,omitempty"`
	GoldModel   string `json:"gold_model,omitempty"`
	GoldRunID   string `json:"gold_run_id,omitempty"`
}

func (r metricScoreRequest) validate() error {
	if r.RecordID <= 0 || strings.TrimSpace(r.Model) == "" {
		return fmt.Errorf("positive record_id and model are required")
	}
	if r.Lang != "en" && r.Lang != "zh-cn" {
		return fmt.Errorf("unsupported language")
	}
	if (r.GoldVersion != "" || r.GoldModel != "" || r.GoldRunID != "") && (r.GoldVersion == "" || r.GoldModel == "" || r.GoldRunID == "") {
		return fmt.Errorf("explicit gold selection requires version, model and run ID")
	}
	return nil
}

func metricScoreIsAdmin(user *ApiTypes.UserInfo) bool {
	if user == nil || strings.TrimSpace(user.UserId) == "" || strings.TrimSpace(user.UserId) == "-" {
		return false
	}
	if user.Admin || user.IsOwner {
		return true
	}
	for _, role := range user.Roles {
		if role = strings.ToLower(strings.TrimSpace(role)); role == "admin" || role == "root" {
			return true
		}
	}
	return false
}

// MetricScoreAdmin protects every scoring endpoint, including history and downloads.
func MetricScoreAdmin(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		rc := EchoFactory.NewFromEcho(c, "20261007-326")
		defer rc.Close()
		user := rc.IsAuthenticated()
		if user == nil {
			return metricScoreHTTPError(c, http.StatusUnauthorized, "UNAUTHORIZED")
		}
		if !metricScoreIsAdmin(user) {
			return metricScoreHTTPError(c, http.StatusForbidden, "FORBIDDEN")
		}
		c.Set("metric-score-user-id", user.UserId)
		c.Set("metric-score-user-name", user.UserName)
		rc.GetLogger().Info("metric scoring request", "method", c.Request().Method, "path", c.Path(), "user_id", user.UserId)
		if ApiTypes.ProjectDBHandle == nil {
			return metricScoreHTTPError(c, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE")
		}
		return next(c)
	}
}

func metricScoreHTTPError(c echo.Context, status int, code string) error {
	return c.JSON(status, map[string]any{"status": false, "error_code": code, "error_msg": code})
}

func ListMetricScoreModels(c echo.Context) error {
	models, err := listMetricReviewModels()
	if err != nil {
		return metricScoreHTTPError(c, http.StatusServiceUnavailable, "MODELS_UNAVAILABLE")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "models": models})
}

func ListMetricScoreGoldRuns(c echo.Context) error {
	recordID, err := strconv.ParseInt(c.QueryParam("record_id"), 10, 64)
	if err != nil || recordID <= 0 {
		return metricScoreHTTPError(c, http.StatusBadRequest, "INVALID_REQUEST")
	}
	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `
SELECT skill_version, model_name, benchmark_run_id, count(*)
FROM testbed.metrics WHERE input_record_id=$1 AND skill_name='extract-metrics-benchmark'
GROUP BY skill_version, model_name, benchmark_run_id ORDER BY skill_version DESC, benchmark_run_id DESC`, recordID)
	if err != nil {
		return metricScoreHTTPError(c, http.StatusInternalServerError, "DATABASE_ERROR")
	}
	defer rows.Close()
	type goldRun struct {
		Version string `json:"skill_version"`
		Model   string `json:"model_name"`
		RunID   string `json:"benchmark_run_id"`
		Rows    int    `json:"rows"`
	}
	runs := []goldRun{}
	for rows.Next() {
		var r goldRun
		if err = rows.Scan(&r.Version, &r.Model, &r.RunID, &r.Rows); err != nil {
			return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
		}
		runs = append(runs, r)
	}
	if rows.Err() != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "runs": runs})
}

func StartMetricScore(c echo.Context) error {
	var req metricScoreRequest
	decoder := json.NewDecoder(io.LimitReader(c.Request().Body, 16384))
	if err := decoder.Decode(&req); err != nil {
		return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
	}
	req.Model = strings.TrimSpace(req.Model)
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
	// Validate configuration before allocating a background run. Never expose config secrets.
	if _, err = loadModelDefByRef(req.Model); err != nil {
		return metricScoreHTTPError(c, 503, "MODEL_UNAVAILABLE")
	}
	if _, err = metricScoreScriptPath(); err != nil {
		return metricScoreHTTPError(c, 503, "SCORER_UNAVAILABLE")
	}
	db := ApiTypes.ProjectDBHandle
	ctx := c.Request().Context()
	title := ""
	if err = db.QueryRowContext(ctx, `SELECT COALESCE(title,'') FROM kb.inputs WHERE id=$1`, req.RecordID).Scan(&title); errors.Is(err, sql.ErrNoRows) {
		return metricScoreHTTPError(c, 404, "DOCUMENT_NOT_FOUND")
	} else if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	if err = expireMetricScoreRuns(ctx, db); err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	requestJSON, _ := json.Marshal(req)
	var id int64
	err = db.QueryRowContext(ctx, `INSERT INTO kb.metric_score_runs
(input_record_id,title,lang,status,model_name,prompt_name,created_by,request)
VALUES ($1,$2,$3,'running',$4,$5,$6,$7)
ON CONFLICT (input_record_id) WHERE status='running' DO NOTHING RETURNING id`, req.RecordID, title, req.Lang, req.Model, metricScorePrompt, c.Get("metric-score-user-name"), string(requestJSON)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var existing int64
		if err = db.QueryRowContext(ctx, `SELECT id FROM kb.metric_score_runs WHERE input_record_id=$1 AND status='running'`, req.RecordID).Scan(&existing); err != nil {
			return metricScoreHTTPError(c, 409, "RUN_CONFLICT")
		}
		run, err := loadMetricScoreRun(ctx, db, existing, true)
		if err != nil {
			return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
		}
		return c.JSON(http.StatusAccepted, map[string]any{"status": true, "run": run})
	}
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	// Once the insert succeeds, the worker must survive request cancellation.
	userID, _ := c.Get("metric-score-user-id").(string)
	go runMetricScore(db, id, req, userID)
	run, err := loadMetricScoreRun(ctx, db, id, true)
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	return c.JSON(http.StatusAccepted, map[string]any{"status": true, "run": run})
}

func expireMetricScoreRuns(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `UPDATE kb.metric_score_runs SET status='failed',error_code='INTERRUPTED',error_msg='INTERRUPTED',finished_at=now()
WHERE status='running' AND created_at < now()-interval '12 minutes'`)
	return err
}

const metricScoreColumns = `id,input_record_id,title,lang,status,model_name,prompt_name,created_by,created_at,finished_at,error_msg,error_code,input_snapshot,matches_snapshot,score_snapshot,report`

type metricScoreScanner interface{ Scan(...any) error }

func scanMetricScoreRun(row metricScoreScanner) (*metricScoreRun, error) {
	var r metricScoreRun
	var inp, matches, score []byte
	err := row.Scan(&r.ID, &r.InputRecordID, &r.Title, &r.Lang, &r.Status, &r.ModelName, &r.PromptName, &r.CreatedBy, &r.CreatedAt, &r.FinishedAt, &r.ErrorMsg, &r.ErrorCode, &inp, &matches, &score, &r.Report)
	r.Input, r.Matches, r.Score = inp, matches, score
	return &r, err
}

func loadMetricScoreRun(ctx context.Context, db *sql.DB, id int64, detail bool) (*metricScoreRun, error) {
	r, err := scanMetricScoreRun(db.QueryRowContext(ctx, "SELECT "+metricScoreColumns+" FROM kb.metric_score_runs WHERE id=$1", id))
	if r != nil && !detail {
		r.Input = nil
		r.Matches = nil
		r.Report = ""
	}
	return r, err
}

func ListMetricScores(c echo.Context) error {
	recordID := int64(0)
	var err error
	if value := c.QueryParam("record_id"); value != "" {
		recordID, err = strconv.ParseInt(value, 10, 64)
		if err != nil || recordID <= 0 {
			return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
		}
	}
	limit, offset := 20, 0
	if s := c.QueryParam("limit"); s != "" {
		limit, err = strconv.Atoi(s)
		if err != nil || limit < 1 || limit > 100 {
			return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
		}
	}
	if s := c.QueryParam("offset"); s != "" {
		offset, err = strconv.Atoi(s)
		if err != nil || offset < 0 {
			return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
		}
	}
	ctx, db := c.Request().Context(), ApiTypes.ProjectDBHandle
	if err = expireMetricScoreRuns(ctx, db); err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	var total int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM kb.metric_score_runs WHERE ($1::bigint=0 OR input_record_id=$1)`, recordID).Scan(&total); err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	// Avoid fetching large evidence snapshots when polling history.
	rows, err := db.QueryContext(ctx, `SELECT id,input_record_id,title,lang,status,model_name,prompt_name,created_by,created_at,finished_at,error_msg,error_code,NULL,NULL,score_snapshot,''
FROM kb.metric_score_runs WHERE ($1::bigint=0 OR input_record_id=$1) ORDER BY id DESC LIMIT $2 OFFSET $3`, recordID, limit, offset)
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	defer rows.Close()
	runs := []*metricScoreRun{}
	for rows.Next() {
		r, err := scanMetricScoreRun(rows)
		if err != nil {
			return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
		}
		runs = append(runs, r)
	}
	if rows.Err() != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "runs": runs, "total": total})
}

func GetMetricScore(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
	}
	ctx, db := c.Request().Context(), ApiTypes.ProjectDBHandle
	if err = expireMetricScoreRuns(ctx, db); err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	run, err := loadMetricScoreRun(ctx, db, id, true)
	if errors.Is(err, sql.ErrNoRows) {
		return metricScoreHTTPError(c, 404, "RUN_NOT_FOUND")
	}
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "run": run})
}

func metricScoreArtifact(run metricScoreRun, kind string) ([]byte, string, string, bool) {
	switch kind {
	case "input":
		return run.Input, "application/json", "input.json", len(run.Input) > 0
	case "matches":
		return run.Matches, "application/json", "matches.json", len(run.Matches) > 0
	case "score":
		return run.Score, "application/json", "score.json", len(run.Score) > 0
	case "report":
		return []byte(run.Report), "text/markdown; charset=utf-8", "report.md", run.Report != ""
	default:
		return nil, "", "", false
	}
}

func DownloadMetricScoreArtifact(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
	}
	run, err := loadMetricScoreRun(c.Request().Context(), ApiTypes.ProjectDBHandle, id, true)
	if errors.Is(err, sql.ErrNoRows) {
		return metricScoreHTTPError(c, 404, "RUN_NOT_FOUND")
	}
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	body, contentType, name, ok := metricScoreArtifact(*run, c.Param("kind"))
	if !ok {
		return metricScoreHTTPError(c, 404, "ARTIFACT_NOT_FOUND")
	}
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="benchmark-%d-%s"`, id, name))
	return c.Blob(http.StatusOK, contentType, body)
}

func runMetricScore(db *sql.DB, id int64, req metricScoreRequest, userID string) {
	logger := loggerutil.CreateDefaultLogger("20261007-785")
	logger.Info("metric benchmark started", "run_id", id, "record_id", req.RecordID, "model", req.Model)
	ctx, cancel := context.WithTimeout(context.Background(), metricReviewRunTimeout)
	defer cancel()
	evidence, err := executeMetricScore(ctx, db, logger, id, req, userID)
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer finishCancel()
	status, code := "done", ""
	if err != nil {
		status = "failed"
		code = metricScoreErrorCode(err)
		logger.Error("metric benchmark failed", "run_id", id, "error_code", code)
	}
	_, storeErr := db.ExecContext(finishCtx, `UPDATE kb.metric_score_runs SET status=$2,model_name=$3,input_snapshot=$4,matches_snapshot=$5,score_snapshot=$6,report=$7,error_msg=$8,error_code=$8,finished_at=now() WHERE id=$1 AND status='running'`,
		id, status, evidence.Model, nullableMetricScoreJSON(evidence.Input), nullableMetricScoreJSON(evidence.Matches), nullableMetricScoreJSON(evidence.Score), evidence.Report, code)
	if storeErr != nil {
		logger.Error("store metric benchmark failed", "run_id", id, "err", storeErr)
	} else {
		logger.Info("metric benchmark finished", "run_id", id, "status", status)
	}
}

func nullableMetricScoreJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return string(b)
}
