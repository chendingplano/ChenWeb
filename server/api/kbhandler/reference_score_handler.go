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
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

// Reference benchmark scoring: the same flow and run shape as the metric benchmark
// (metric_score_handler.go), over the gold references of the extract-references-benchmark skill
// and the score-extract-references scorer. Runs are stored in kb.reference_score_runs.

const referenceScorePrompt = "prompt-score-extract-references-v1.md"

var referenceScoreKind = scoreKind{"REFERENCE_BENCHMARK_SCORER_PATH", "score-extract-references", "extract-references-benchmark",
	"reference-score-runs", referenceScorePrompt, "score_extract_references", "MID-20261014-01", "reference"}

func ListReferenceScoreGoldRuns(c echo.Context) error {
	recordID, err := strconv.ParseInt(c.QueryParam("record_id"), 10, 64)
	if err != nil || recordID <= 0 {
		return metricScoreHTTPError(c, http.StatusBadRequest, "INVALID_REQUEST")
	}
	// Read from the audit table so a run with zero gold references is still listed.
	rows, err := ApiTypes.ProjectDBHandle.QueryContext(c.Request().Context(), `
SELECT r.skill_version, r.model_name, r.run_id,
  (SELECT count(*) FROM testbed.ref_mentions m
    WHERE (m.skill_name, m.skill_version, m.model_name, m.run_id) = (r.skill_name, r.skill_version, r.model_name, r.run_id))
FROM testbed.reference_benchmark_runs r
WHERE r.input_record_id=$1 AND r.skill_name='extract-references-benchmark'
ORDER BY r.skill_version DESC, r.run_id DESC`, recordID)
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

func StartReferenceScore(c echo.Context) error {
	var req metricScoreRequest
	if err := json.NewDecoder(io.LimitReader(c.Request().Body, 16384)).Decode(&req); err != nil {
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
	if _, err = scoreSkillScriptPath(referenceScoreKind.scriptEnv, referenceScoreKind.scoreSkill); err != nil {
		return metricScoreHTTPError(c, 503, "SCORER_UNAVAILABLE")
	}
	db, ctx := ApiTypes.ProjectDBHandle, c.Request().Context()
	title := ""
	if err = db.QueryRowContext(ctx, `SELECT COALESCE(title,'') FROM kb.inputs WHERE id=$1`, req.RecordID).Scan(&title); errors.Is(err, sql.ErrNoRows) {
		return metricScoreHTTPError(c, 404, "DOCUMENT_NOT_FOUND")
	} else if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	if err = expireReferenceScoreRuns(ctx, db); err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	requestJSON, _ := json.Marshal(req)
	var id int64
	err = db.QueryRowContext(ctx, `INSERT INTO kb.reference_score_runs
(input_record_id,title,lang,status,model_name,prompt_name,created_by,request)
VALUES ($1,$2,$3,'running',$4,$5,$6,$7)
ON CONFLICT (input_record_id) WHERE status='running' DO NOTHING RETURNING id`, req.RecordID, title, req.Lang, req.Model, referenceScorePrompt, c.Get("metric-score-user-name"), string(requestJSON)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var existing int64
		if err = db.QueryRowContext(ctx, `SELECT id FROM kb.reference_score_runs WHERE input_record_id=$1 AND status='running'`, req.RecordID).Scan(&existing); err != nil {
			return metricScoreHTTPError(c, 409, "RUN_CONFLICT")
		}
		run, err := loadReferenceScoreRun(ctx, db, existing)
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
	go runReferenceScore(db, id, req, userID)
	run, err := loadReferenceScoreRun(ctx, db, id)
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	return c.JSON(http.StatusAccepted, map[string]any{"status": true, "run": run})
}

func expireReferenceScoreRuns(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `UPDATE kb.reference_score_runs SET status='failed',error_code='INTERRUPTED',error_msg='INTERRUPTED',finished_at=now()
WHERE status='running' AND created_at < now()-interval '12 minutes'`)
	return err
}

func loadReferenceScoreRun(ctx context.Context, db *sql.DB, id int64) (*metricScoreRun, error) {
	return scanMetricScoreRun(db.QueryRowContext(ctx, "SELECT "+metricScoreColumns+" FROM kb.reference_score_runs WHERE id=$1", id))
}

func ListReferenceScores(c echo.Context) error {
	recordID, limit, offset := int64(0), 20, 0
	var err error
	if value := c.QueryParam("record_id"); value != "" {
		if recordID, err = strconv.ParseInt(value, 10, 64); err != nil || recordID <= 0 {
			return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
		}
	}
	if s := c.QueryParam("limit"); s != "" {
		if limit, err = strconv.Atoi(s); err != nil || limit < 1 || limit > 100 {
			return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
		}
	}
	if s := c.QueryParam("offset"); s != "" {
		if offset, err = strconv.Atoi(s); err != nil || offset < 0 {
			return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
		}
	}
	ctx, db := c.Request().Context(), ApiTypes.ProjectDBHandle
	if err = expireReferenceScoreRuns(ctx, db); err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	var total int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM kb.reference_score_runs WHERE ($1::bigint=0 OR input_record_id=$1)`, recordID).Scan(&total); err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	// Avoid fetching large evidence snapshots when polling history.
	rows, err := db.QueryContext(ctx, `SELECT id,input_record_id,title,lang,status,model_name,prompt_name,created_by,created_at,finished_at,error_msg,error_code,NULL,NULL,score_snapshot,''
FROM kb.reference_score_runs WHERE ($1::bigint=0 OR input_record_id=$1) ORDER BY id DESC LIMIT $2 OFFSET $3`, recordID, limit, offset)
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

func GetReferenceScore(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
	}
	ctx, db := c.Request().Context(), ApiTypes.ProjectDBHandle
	if err = expireReferenceScoreRuns(ctx, db); err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	run, err := loadReferenceScoreRun(ctx, db, id)
	if errors.Is(err, sql.ErrNoRows) {
		return metricScoreHTTPError(c, 404, "RUN_NOT_FOUND")
	}
	if err != nil {
		return metricScoreHTTPError(c, 500, "DATABASE_ERROR")
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "run": run})
}

func DownloadReferenceScoreArtifact(c echo.Context) error {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		return metricScoreHTTPError(c, 400, "INVALID_REQUEST")
	}
	run, err := loadReferenceScoreRun(c.Request().Context(), ApiTypes.ProjectDBHandle, id)
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
	c.Response().Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="reference-benchmark-%d-%s"`, id, name))
	return c.Blob(http.StatusOK, contentType, body)
}

func runReferenceScore(db *sql.DB, id int64, req metricScoreRequest, userID string) {
	logger := loggerutil.CreateDefaultLogger("20261014-418")
	logger.Info("reference benchmark started", "run_id", id, "record_id", req.RecordID, "model", req.Model)
	ctx, cancel := context.WithTimeout(context.Background(), metricReviewRunTimeout)
	defer cancel()
	evidence, err := executeScore(ctx, db, logger, referenceScoreKind, id, req, userID)
	finishCtx, finishCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer finishCancel()
	status, code := "done", ""
	if err != nil {
		status = "failed"
		code = metricScoreErrorCode(err)
		logger.Error("reference benchmark failed", "run_id", id, "error_code", code)
	}
	_, storeErr := db.ExecContext(finishCtx, `UPDATE kb.reference_score_runs SET status=$2,model_name=$3,input_snapshot=$4,matches_snapshot=$5,score_snapshot=$6,report=$7,error_msg=$8,error_code=$8,finished_at=now() WHERE id=$1 AND status='running'`,
		id, status, evidence.Model, nullableMetricScoreJSON(evidence.Input), nullableMetricScoreJSON(evidence.Matches), nullableMetricScoreJSON(evidence.Score), evidence.Report, code)
	if storeErr != nil {
		logger.Error("store reference benchmark failed", "run_id", id, "err", storeErr)
	} else {
		logger.Info("reference benchmark finished", "run_id", id, "status", status)
	}
}
