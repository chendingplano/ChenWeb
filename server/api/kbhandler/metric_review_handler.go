package kbhandler

// LLM review of one record's extracted kb.metrics (System Admin -> LLM ->
// Review Metrics). See openspec/changes/llm-review-metrics for the design:
// reviews are stored in kb.metric_reviews (one row per run, newest = current),
// run in the background, and are polled by the page. Reviews are per language
// and can be translated from another language (openspec change
// metric-review-i18n-export).

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
	"github.com/labstack/echo/v4"
)

const (
	// metricReviewMaxInputChars caps the assembled LLM input (design D4). Larger
	// documents fail explicitly instead of being truncated.
	metricReviewMaxInputChars = 600000
	// metricReviewRunTimeout bounds one background run. A 'running' row older
	// than this is reported as failed (interrupted), e.g. after a server restart.
	metricReviewRunTimeout = 10 * time.Minute

	metricReviewStatusRunning = "running"
	metricReviewStatusDone    = "done"
	metricReviewStatusFailed  = "failed"

	metricReviewDefaultLang = "en"
)

// metricReviewLangNames are the supported report languages (the paraglide UI
// locales) and the names given to the LLM.
var metricReviewLangNames = map[string]string{
	"en":    "English",
	"zh-cn": "Simplified Chinese",
}

// normalizeMetricReviewLang maps a request language to a supported code. Empty
// means English, which keeps pre-i18n clients working.
func normalizeMetricReviewLang(s string) (string, bool) {
	v := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "_", "-")
	switch v {
	case "":
		return metricReviewDefaultLang, true
	case "zh", "zh-hans":
		v = "zh-cn"
	}
	_, ok := metricReviewLangNames[v]
	return v, ok
}

// metricReviewRow is one kb.metric_reviews row as returned to the page.
type metricReviewRow struct {
	ID            int64           `json:"id"`
	InputRecordID int64           `json:"input_record_id"`
	Lang          string          `json:"lang"`
	Status        string          `json:"status"`
	Report        json.RawMessage `json:"report,omitempty"`
	ErrorMsg      string          `json:"error_msg,omitempty"`
	ModelName     string          `json:"model_name,omitempty"`
	PromptName    string          `json:"prompt_name,omitempty"`
	MetricsCount  int             `json:"metrics_count"`
	CreatedBy     string          `json:"created_by,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	FinishedAt    *time.Time      `json:"finished_at,omitempty"`
	// TranslatedFromID is the review this one was translated from (nil for an
	// LLM review).
	TranslatedFromID *int64 `json:"translated_from_id,omitempty"`
}

type metricReviewResponse struct {
	Status bool             `json:"status"`
	Review *metricReviewRow `json:"review"`
	// Started is true when the POST launched a new LLM run.
	Started bool `json:"started,omitempty"`
	// OtherLangs (GET only) lists the other languages that have a done review,
	// so the page can offer a translation when Review is nil or failed.
	OtherLangs []string `json:"other_langs,omitempty"`
}

// metricReviewReport is the stored report (design D5).
type metricReviewReport struct {
	Summary         string                       `json:"summary"`
	Tally           metricReviewTally            `json:"tally"`
	MissedMetrics   []metricReviewMissed         `json:"missed_metrics"`
	NonMetrics      []metricReviewNonMetric      `json:"non_metrics"`
	AttributeIssues []metricReviewAttributeIssue `json:"attribute_issues"`
	Recommendations []string                     `json:"recommendations"`
	Metrics         []metricReviewSnapshot       `json:"metrics"`
}

type metricReviewTally struct {
	Stored       int `json:"stored"`
	Kept         int `json:"kept"`
	NotMetric    int `json:"not_metric"`
	Duplicate    int `json:"duplicate"`
	FormulaInput int `json:"formula_input"`
	Missed       int `json:"missed"`
}

type metricReviewMissed struct {
	Lines    flexString `json:"lines"`
	Name     flexString `json:"name"`
	Value    flexString `json:"value"`
	Unit     flexString `json:"unit"`
	Reason   flexString `json:"reason"`
	Severity string     `json:"severity"`
}

type metricReviewNonMetric struct {
	MetricIDs   []string   `json:"metric_ids"`
	Category    string     `json:"category"`
	DuplicateOf flexString `json:"duplicate_of,omitempty"`
	Reason      flexString `json:"reason"`
}

type metricReviewAttributeIssue struct {
	MetricIDs []string   `json:"metric_ids"`
	Field     flexString `json:"field"`
	Stored    flexString `json:"stored"`
	Suggested flexString `json:"suggested"`
	Reason    flexString `json:"reason"`
	Severity  string     `json:"severity"`
}

// metricReviewSnapshot records the reviewed metric as it was at review time, so
// the report stays readable after later re-extraction changes kb.metrics.
type metricReviewSnapshot struct {
	MetricID string `json:"metric_id"`
	Name     string `json:"name"`
	Value    string `json:"value,omitempty"`
	Unit     string `json:"unit,omitempty"`
	Lines    string `json:"lines,omitempty"`
}

// metricReviewLLMOutput is the raw model output shape (prompt schema).
type metricReviewLLMOutput struct {
	Summary         flexString                   `json:"summary"`
	MissedMetrics   []metricReviewMissed         `json:"missed_metrics"`
	NonMetrics      []metricReviewNonMetric      `json:"non_metrics"`
	AttributeIssues []metricReviewAttributeIssue `json:"attribute_issues"`
	Recommendations []flexString                 `json:"recommendations"`
}

// flexString accepts a JSON string, number, or bool (models sometimes emit
// line numbers or values as numbers) and stores it as a string.
type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = flexString(strings.TrimSpace(s))
		return nil
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v == nil {
		*f = ""
		return nil
	}
	*f = flexString(strings.TrimSpace(fmt.Sprint(v)))
	return nil
}

// metricReviewInputMetric is one kb.metrics row as sent to the LLM (design D4).
type metricReviewInputMetric struct {
	MetricID             string          `json:"metric_id"`
	SourceLineSpans      json.RawMessage `json:"source_line_spans,omitempty"`
	MetricName           string          `json:"metric_name,omitempty"`
	MetricSubject        string          `json:"metric_subject,omitempty"`
	MetricValue          string          `json:"metric_value,omitempty"`
	MetricUnit           string          `json:"metric_unit,omitempty"`
	ThresholdOrTarget    string          `json:"threshold_or_target,omitempty"`
	ValueMin             *float64        `json:"value_min,omitempty"`
	ValueMax             *float64        `json:"value_max,omitempty"`
	ValueDataType        string          `json:"value_data_type,omitempty"`
	ValueRangeType       string          `json:"value_range_type,omitempty"`
	ValueClass           string          `json:"value_class,omitempty"`
	Condition            string          `json:"condition,omitempty"`
	IsExplicitMetric     *bool           `json:"is_explicit_metric,omitempty"`
	Confidence           *float64        `json:"confidence,omitempty"`
	LocationType         string          `json:"location_type,omitempty"`
	MeasurementFrequency string          `json:"measurement_frequency,omitempty"`
	FormulaOrDefinition  string          `json:"formula_or_definition,omitempty"`
	CreatedDate          string          `json:"created_date,omitempty"`
}

type metricReviewDocHeader struct {
	RecordID       int64
	Title          string
	DocNo          string
	ResultFilename string
}

// GetMetricReview handles GET /api/v1/kb/metric-reviews/:record_id. It returns
// the record's newest review (never calls the LLM), or review=null.
func GetMetricReview(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_MRV_100")
	defer rc.Close()
	logger := rc.GetLogger()

	recordID, err := strconv.ParseInt(strings.TrimSpace(c.Param("record_id")), 10, 64)
	if err != nil || recordID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid record_id (CWB_KB_MRV_110)"})
	}
	lang, ok := normalizeMetricReviewLang(c.QueryParam("lang"))
	if !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "unsupported lang (CWB_KB_MRV_111)"})
	}
	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle
	row, err := loadLatestMetricReview(ctx, db, recordID, lang)
	if err != nil {
		logger.Error("load latest metric review failed", "record_id", recordID, "lang", lang, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load review (CWB_KB_MRV_120)"})
	}
	if row != nil {
		applyStaleRunningStatus(row, time.Now())
	}
	resp := metricReviewResponse{Status: true, Review: row}
	// Offer a translation when there is no usable review in lang: none, or the
	// latest one failed (e.g. a failed translation, which must stay retryable).
	if row == nil || row.Status == metricReviewStatusFailed {
		if resp.OtherLangs, err = loadOtherMetricReviewLangs(ctx, db, recordID, lang); err != nil {
			logger.Error("load other review languages failed", "record_id", recordID, "lang", lang, "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load review (CWB_KB_MRV_121)"})
		}
	}
	return c.JSON(http.StatusOK, resp)
}

type startMetricReviewRequest struct {
	Force bool   `json:"force"`
	Lang  string `json:"lang"`
}

// StartMetricReview handles POST /api/v1/kb/metric-reviews/:record_id with body
// {"force": bool, "lang": "en"|"zh-cn"}. Within that language it returns an existing review when design D3's cache rules
// allow it; otherwise it inserts a 'running' row and starts the LLM review in
// the background.
func StartMetricReview(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_MRV_200")
	defer rc.Close()
	logger := rc.GetLogger()
	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle

	recordID, err := strconv.ParseInt(strings.TrimSpace(c.Param("record_id")), 10, 64)
	if err != nil || recordID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid record_id (CWB_KB_MRV_210)"})
	}
	var req startMetricReviewRequest
	if c.Request().ContentLength != 0 {
		if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
			return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_MRV_211)"})
		}
	}
	lang, ok := normalizeMetricReviewLang(req.Lang)
	if !ok {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "unsupported lang (CWB_KB_MRV_212)"})
	}

	latest, err := loadLatestMetricReview(ctx, db, recordID, lang)
	if err != nil {
		logger.Error("load latest metric review failed", "record_id", recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load review (CWB_KB_MRV_220)"})
	}
	now := time.Now()
	if latest != nil {
		applyStaleRunningStatus(latest, now)
	}
	if !shouldStartMetricReview(latest, req.Force) {
		logger.Info("returning existing metric review", "record_id", recordID, "lang", lang, "review_id", latest.ID, "status", latest.Status, "force", req.Force)
		return c.JSON(http.StatusOK, metricReviewResponse{Status: true, Review: latest})
	}

	var metricsCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM kb.metrics WHERE input_record_id = $1`, recordID).Scan(&metricsCount); err != nil {
		logger.Error("count metrics failed", "record_id", recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to count metrics (CWB_KB_MRV_230)"})
	}
	if metricsCount == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("record %d has no extracted metrics (CWB_KB_MRV_231)", recordID)})
	}

	var userName, userID string
	if user := rc.IsAuthenticated(); user != nil {
		userName, userID = user.UserName, user.UserId
	}
	row := &metricReviewRow{InputRecordID: recordID, Lang: lang, Status: metricReviewStatusRunning, MetricsCount: metricsCount, CreatedBy: userName}
	if err := db.QueryRowContext(ctx, `
INSERT INTO kb.metric_reviews (input_record_id, lang, status, metrics_count, created_by)
VALUES ($1, $2, $3, $4, NULLIF($5, ''))
RETURNING id, created_at`, recordID, lang, metricReviewStatusRunning, metricsCount, userName).Scan(&row.ID, &row.CreatedAt); err != nil {
		logger.Error("insert metric review failed", "record_id", recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create review (CWB_KB_MRV_240)"})
	}
	logger.Info("starting metric review", "record_id", recordID, "lang", lang, "review_id", row.ID, "metrics_count", metricsCount, "force", req.Force)

	go runMetricReview(db, row.ID, recordID, lang, userID)

	return c.JSON(http.StatusOK, metricReviewResponse{Status: true, Review: row, Started: true})
}

// shouldStartMetricReview applies design D3: a live running review is always
// reused; a done review is reused unless force; otherwise start a new run.
func shouldStartMetricReview(latest *metricReviewRow, force bool) bool {
	if latest == nil {
		return true
	}
	switch latest.Status {
	case metricReviewStatusRunning:
		return false
	case metricReviewStatusDone:
		return force
	default:
		return true
	}
}

// applyStaleRunningStatus reports a 'running' row older than the run timeout as
// failed (interrupted). It only changes the returned value, not the DB row.
func applyStaleRunningStatus(row *metricReviewRow, now time.Time) {
	if row.Status == metricReviewStatusRunning && now.Sub(row.CreatedAt) > metricReviewRunTimeout {
		row.Status = metricReviewStatusFailed
		row.ErrorMsg = "review interrupted (did not finish within the run timeout)"
	}
}

const metricReviewSelectCols = `
SELECT id, input_record_id, lang, status, report, error_msg, model_name, prompt_name,
       metrics_count, created_by, created_at, finished_at, translated_from_id
FROM kb.metric_reviews`

// loadLatestMetricReview returns the record's newest review in lang, or nil.
func loadLatestMetricReview(ctx context.Context, db *sql.DB, recordID int64, lang string) (*metricReviewRow, error) {
	return scanMetricReviewRow(db.QueryRowContext(ctx, metricReviewSelectCols+`
WHERE input_record_id = $1 AND lang = $2
ORDER BY created_at DESC, id DESC
LIMIT 1`, recordID, lang))
}

// loadTranslationSource returns the record's newest done review in any
// language other than lang (the source of a translation), or nil.
func loadTranslationSource(ctx context.Context, db *sql.DB, recordID int64, lang string) (*metricReviewRow, error) {
	return scanMetricReviewRow(db.QueryRowContext(ctx, metricReviewSelectCols+`
WHERE input_record_id = $1 AND lang <> $2 AND status = $3 AND report IS NOT NULL
ORDER BY created_at DESC, id DESC
LIMIT 1`, recordID, lang, metricReviewStatusDone))
}

// loadOtherMetricReviewLangs lists the languages other than lang that have a
// done review for the record.
func loadOtherMetricReviewLangs(ctx context.Context, db *sql.DB, recordID int64, lang string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
SELECT DISTINCT lang FROM kb.metric_reviews
WHERE input_record_id = $1 AND lang <> $2 AND status = $3 AND report IS NOT NULL
ORDER BY lang`, recordID, lang, metricReviewStatusDone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var l string
		if err := rows.Scan(&l); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func scanMetricReviewRow(sc *sql.Row) (*metricReviewRow, error) {
	var (
		row                                      metricReviewRow
		report                                   []byte
		errMsg, modelName, promptName, createdBy sql.NullString
		finishedAt                               sql.NullTime
		translatedFrom                           sql.NullInt64
	)
	err := sc.Scan(&row.ID, &row.InputRecordID, &row.Lang, &row.Status, &report, &errMsg, &modelName,
		&promptName, &row.MetricsCount, &createdBy, &row.CreatedAt, &finishedAt, &translatedFrom)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(report) > 0 {
		row.Report = json.RawMessage(report)
	}
	row.ErrorMsg, row.ModelName, row.PromptName, row.CreatedBy = errMsg.String, modelName.String, promptName.String, createdBy.String
	if finishedAt.Valid {
		row.FinishedAt = &finishedAt.Time
	}
	if translatedFrom.Valid {
		row.TranslatedFromID = &translatedFrom.Int64
	}
	return &row, nil
}

// runMetricReview is the background run. It owns its context and logger
// because the HTTP request that started it has already returned.
func runMetricReview(db *sql.DB, reviewID, recordID int64, lang, userID string) {
	logger := loggerutil.CreateDefaultLogger("20260929-714")
	ctx, cancel := context.WithTimeout(context.Background(), metricReviewRunTimeout)
	defer cancel()

	report, modelName, promptName, err := executeMetricReview(ctx, db, logger, recordID, lang, userID)
	finishMetricReview(db, logger, reviewID, recordID, report, modelName, promptName, err)
}

// finishMetricReview stores the outcome of a background run (review or
// translation) on its kb.metric_reviews row.
func finishMetricReview(db *sql.DB, logger ApiTypes.JimoLogger, reviewID, recordID int64, report metricReviewReport, modelName, promptName string, err error) {
	if err != nil {
		logger.Error("metric review failed", "review_id", reviewID, "record_id", recordID, "err", err)
		if _, uerr := db.Exec(`
UPDATE kb.metric_reviews SET status = $2, error_msg = $3, model_name = NULLIF($4, ''), prompt_name = NULLIF($5, ''), finished_at = now()
WHERE id = $1`, reviewID, metricReviewStatusFailed, err.Error(), modelName, promptName); uerr != nil {
			logger.Error("mark metric review failed: update failed", "review_id", reviewID, "err", uerr)
		}
		return
	}
	reportJSON, err := json.Marshal(report)
	if err != nil {
		logger.Error("marshal metric review report failed", "review_id", reviewID, "err", err)
		return
	}
	if _, err := db.Exec(`
UPDATE kb.metric_reviews SET status = $2, report = $3, model_name = $4, prompt_name = $5, finished_at = now()
WHERE id = $1`, reviewID, metricReviewStatusDone, reportJSON, modelName, promptName); err != nil {
		logger.Error("store metric review report failed", "review_id", reviewID, "err", err)
		return
	}
	logger.Info("metric review done", "review_id", reviewID, "record_id", recordID, "model_name", modelName,
		"stored", report.Tally.Stored, "kept", report.Tally.Kept, "missed", report.Tally.Missed)
}

// newMetricReviewLLMClient builds the client for the model named by
// REVIEW_METRICS_MODEL_NAME (used by both reviews and translations).
func newMetricReviewLLMClient(logger ApiTypes.JimoLogger) (ApiTypes.LLMModelDef, *llmclients.OpenAIJSONClient, error) {
	modelRef, cfg, err := loadWikiModelDef("REVIEW_METRICS_MODEL_NAME")
	if err != nil {
		return cfg, nil, fmt.Errorf("load model named by REVIEW_METRICS_MODEL_NAME (CWB_KB_MRV_302): %w", err)
	}
	if modelRef == "" {
		return cfg, nil, fmt.Errorf("missing env var REVIEW_METRICS_MODEL_NAME (CWB_KB_MRV_303)")
	}
	client, err := llmclients.NewOpenAIJSONClientFromConfig(llmclients.OpenAIJSONClientConfig{
		ModelName:            cfg.ModelName,
		APIKey:               cfg.APIKey,
		BaseURL:              cfg.BaseURL,
		ProfileName:          modelRef,
		TimeoutSec:           cfg.TimeoutSec,
		ThinkingType:         cfg.ThinkingType,
		MaxOutputTokens:      cfg.MaxOutputTokens,
		OmitTemperature:      cfg.OmitTemperature,
		MaxInflight:          cfg.MaxInflight,
		MaxRequestsPerMinute: cfg.MaxRequestsPerMinute,
		MaxTokensPerMinute:   cfg.MaxTokensPerMinute,
		TokenReservePerCall:  cfg.TokenReservePerCall,
	}, logger)
	if err != nil {
		return cfg, nil, fmt.Errorf("create LLM client failed (CWB_KB_MRV_320): %w", err)
	}
	return cfg, client, nil
}

// loadMetricReviewPrompt reads the prompt file named by envVar from prompts/.
func loadMetricReviewPrompt(envVar, missingLoc, unreadableLoc string) (string, string, error) {
	promptName := strings.TrimSpace(os.Getenv(envVar))
	if promptName == "" {
		return "", "", fmt.Errorf("missing env var %s (%s)", envVar, missingLoc)
	}
	promptBytes, err := os.ReadFile(filepath.Join("prompts", promptName))
	if err != nil || strings.TrimSpace(string(promptBytes)) == "" {
		return "", promptName, fmt.Errorf("cannot read prompt prompts/%s named by %s (%s): %v", promptName, envVar, unreadableLoc, err)
	}
	return strings.TrimSpace(string(promptBytes)), promptName, nil
}

// executeMetricReview loads the prompt, model, document and metrics, calls the
// LLM, and returns the post-processed report.
func executeMetricReview(ctx context.Context, db *sql.DB, logger ApiTypes.JimoLogger, recordID int64, lang, userID string) (metricReviewReport, string, string, error) {
	promptText, promptName, err := loadMetricReviewPrompt("REVIEW_METRICS_PROMPT", "CWB_KB_MRV_300", "CWB_KB_MRV_301")
	if err != nil {
		return metricReviewReport{}, "", promptName, err
	}
	cfg, client, err := newMetricReviewLLMClient(logger)
	if err != nil {
		return metricReviewReport{}, cfg.ModelName, promptName, err
	}

	header, err := loadMetricReviewDocHeader(ctx, db, recordID)
	if err != nil {
		return metricReviewReport{}, cfg.ModelName, promptName, err
	}
	if strings.TrimSpace(header.ResultFilename) == "" {
		return metricReviewReport{}, cfg.ModelName, promptName, fmt.Errorf("record %d has no parsed document (result_filename is empty) (CWB_KB_MRV_310)", recordID)
	}
	lines, err := readRawLinesFile(rawLinePathFor(header.ResultFilename))
	if err != nil {
		return metricReviewReport{}, cfg.ModelName, promptName, fmt.Errorf("read document lines failed (CWB_KB_MRV_311): %w", err)
	}
	metrics, err := loadMetricReviewInputMetrics(ctx, db, recordID)
	if err != nil {
		return metricReviewReport{}, cfg.ModelName, promptName, fmt.Errorf("load metrics failed (CWB_KB_MRV_312): %w", err)
	}
	if len(metrics) == 0 {
		return metricReviewReport{}, cfg.ModelName, promptName, fmt.Errorf("record %d has no extracted metrics (CWB_KB_MRV_313)", recordID)
	}

	inputText, err := buildMetricReviewInput(header, lang, lines, metrics)
	if err != nil {
		return metricReviewReport{}, cfg.ModelName, promptName, err
	}

	logger.Info("calling LLM for metric review", "record_id", recordID, "lang", lang, "model_name", cfg.ModelName, "input_chars", len(inputText), "metrics", len(metrics))
	payload, err := client.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		UserID:     userID,
		PromptName: promptName,
		PromptText: promptText,
		ModelName:  cfg.ModelName,
		InputText:  inputText,
		RecordID:   recordID,
		CallReason: "review_metrics",
		CallLoc:    "MID-20260929-05",
	})
	if err != nil {
		return metricReviewReport{}, cfg.ModelName, promptName, fmt.Errorf("LLM call failed (CWB_KB_MRV_321): %w", err)
	}
	report, dropped, err := finalizeMetricReview(payload, metrics)
	if err != nil {
		return metricReviewReport{}, cfg.ModelName, promptName, err
	}
	if len(dropped) > 0 {
		logger.Warn("metric review cited unknown metric ids; dropped", "record_id", recordID, "dropped", dropped)
	}
	return report, cfg.ModelName, promptName, nil
}

func loadMetricReviewDocHeader(ctx context.Context, db *sql.DB, recordID int64) (metricReviewDocHeader, error) {
	inputTable, err := resolveInputTable(db)
	if err != nil {
		return metricReviewDocHeader{}, fmt.Errorf("resolve input table failed (CWB_KB_MRV_330): %w", err)
	}
	h := metricReviewDocHeader{RecordID: recordID}
	var title, docNo, resultFile sql.NullString
	q := fmt.Sprintf(`SELECT title, doc_no, result_filename FROM %s WHERE id = $1`, inputTable)
	if err := db.QueryRowContext(ctx, q, recordID).Scan(&title, &docNo, &resultFile); err != nil {
		if err == sql.ErrNoRows {
			return h, fmt.Errorf("record %d not found (CWB_KB_MRV_331)", recordID)
		}
		return h, fmt.Errorf("load record %d failed (CWB_KB_MRV_332): %w", recordID, err)
	}
	h.Title, h.DocNo, h.ResultFilename = title.String, docNo.String, resultFile.String
	return h, nil
}

func loadMetricReviewInputMetrics(ctx context.Context, db *sql.DB, recordID int64) ([]metricReviewInputMetric, error) {
	rows, err := db.QueryContext(ctx, `
SELECT COALESCE(metric_id, 'id:' || id::text), source_line_spans,
       COALESCE(metric_name, ''), COALESCE(metric_subject, ''), COALESCE(metric_value, ''),
       COALESCE(metric_unit, ''), COALESCE(threshold_or_target, ''), value_min, value_max,
       COALESCE(value_data_type, ''), COALESCE(value_range_type, ''), COALESCE(value_class, ''),
       COALESCE(condition, ''), is_explicit_metric, confidence, COALESCE(location_type, ''),
       COALESCE(measurement_frequency, ''), COALESCE(formula_or_definition, ''),
       to_char(created_at, 'YYYY-MM-DD')
FROM kb.metrics
WHERE input_record_id = $1
ORDER BY id`, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]metricReviewInputMetric, 0, 64)
	for rows.Next() {
		var (
			m                  metricReviewInputMetric
			spans              []byte
			valueMin, valueMax sql.NullFloat64
			explicit           sql.NullBool
			confidence         sql.NullFloat64
		)
		if err := rows.Scan(&m.MetricID, &spans, &m.MetricName, &m.MetricSubject, &m.MetricValue,
			&m.MetricUnit, &m.ThresholdOrTarget, &valueMin, &valueMax, &m.ValueDataType,
			&m.ValueRangeType, &m.ValueClass, &m.Condition, &explicit, &confidence, &m.LocationType,
			&m.MeasurementFrequency, &m.FormulaOrDefinition, &m.CreatedDate); err != nil {
			return nil, err
		}
		if len(spans) > 0 {
			m.SourceLineSpans = json.RawMessage(spans)
		}
		if valueMin.Valid {
			m.ValueMin = &valueMin.Float64
		}
		if valueMax.Valid {
			m.ValueMax = &valueMax.Float64
		}
		if explicit.Valid {
			m.IsExplicitMetric = &explicit.Bool
		}
		if confidence.Valid {
			m.Confidence = &confidence.Float64
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// buildMetricReviewInput renders the LLM input (design D4): document header,
// output language, source lines as "L<n>\t<type>\t<text>", then the metrics as JSON. It fails
// rather than truncating when the result exceeds metricReviewMaxInputChars.
func buildMetricReviewInput(h metricReviewDocHeader, lang string, lines []rawLine, metrics []metricReviewInputMetric) (string, error) {
	var b strings.Builder
	b.WriteString("DOCUMENT\n")
	fmt.Fprintf(&b, "record_id: %d\ntitle: %s\ndoc_no: %s\noutput_language: %s\n\n", h.RecordID, h.Title, h.DocNo, metricReviewLangLabel(lang))
	for _, ln := range lines {
		fmt.Fprintf(&b, "L%d\t%s\t%s\n", ln.LineNumber, ln.LineType, ln.Content)
	}
	metricsJSON, err := json.MarshalIndent(metrics, "", " ")
	if err != nil {
		return "", fmt.Errorf("marshal metrics failed (CWB_KB_MRV_340): %w", err)
	}
	b.WriteString("\nMETRICS\n")
	b.Write(metricsJSON)
	b.WriteString("\n")

	text := b.String()
	if n := len([]rune(text)); n > metricReviewMaxInputChars {
		return "", fmt.Errorf("document too large to review: %d characters, limit is %d (CWB_KB_MRV_341)", n, metricReviewMaxInputChars)
	}
	return text, nil
}

// finalizeMetricReview turns the LLM payload into the stored report (design
// D5): unknown metric IDs are dropped (and returned), entries left without IDs
// are removed, category/severity are normalised, the tally is computed here
// rather than trusted from the model, and a metrics snapshot is attached.
func finalizeMetricReview(payload map[string]any, metrics []metricReviewInputMetric) (metricReviewReport, []string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return metricReviewReport{}, nil, fmt.Errorf("re-encode LLM output failed (CWB_KB_MRV_350): %w", err)
	}
	var out metricReviewLLMOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return metricReviewReport{}, nil, fmt.Errorf("LLM output does not match the review schema (CWB_KB_MRV_351): %w", err)
	}

	known := make(map[string]bool, len(metrics))
	snapshot := make([]metricReviewSnapshot, 0, len(metrics))
	for _, m := range metrics {
		known[m.MetricID] = true
		snapshot = append(snapshot, metricReviewSnapshot{
			MetricID: m.MetricID, Name: m.MetricName, Value: m.MetricValue, Unit: m.MetricUnit,
			Lines: formatMetricReviewSpans(m.SourceLineSpans),
		})
	}
	droppedSet := map[string]bool{}
	filterIDs := func(ids []string) []string {
		kept := make([]string, 0, len(ids))
		seen := map[string]bool{}
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			if !known[id] {
				droppedSet[id] = true
				continue
			}
			seen[id] = true
			kept = append(kept, id)
		}
		return kept
	}

	report := metricReviewReport{
		Summary:         string(out.Summary),
		MissedMetrics:   make([]metricReviewMissed, 0, len(out.MissedMetrics)),
		NonMetrics:      make([]metricReviewNonMetric, 0, len(out.NonMetrics)),
		AttributeIssues: make([]metricReviewAttributeIssue, 0, len(out.AttributeIssues)),
		Recommendations: make([]string, 0, len(out.Recommendations)),
		Metrics:         snapshot,
	}
	for _, m := range out.MissedMetrics {
		m.Severity = normalizeMetricReviewSeverity(m.Severity)
		report.MissedMetrics = append(report.MissedMetrics, m)
	}

	removed := map[string]string{} // metric_id -> first category it was removed under
	for _, nm := range out.NonMetrics {
		nm.MetricIDs = filterIDs(nm.MetricIDs)
		if len(nm.MetricIDs) == 0 {
			continue
		}
		nm.Category = normalizeMetricReviewCategory(nm.Category)
		if nm.Category != "duplicate" || !known[string(nm.DuplicateOf)] {
			nm.DuplicateOf = ""
		}
		for _, id := range nm.MetricIDs {
			if _, ok := removed[id]; !ok {
				removed[id] = nm.Category
			}
		}
		report.NonMetrics = append(report.NonMetrics, nm)
	}
	for _, ai := range out.AttributeIssues {
		ai.MetricIDs = filterIDs(ai.MetricIDs)
		if len(ai.MetricIDs) == 0 {
			continue
		}
		ai.Severity = normalizeMetricReviewSeverity(ai.Severity)
		report.AttributeIssues = append(report.AttributeIssues, ai)
	}
	for _, r := range out.Recommendations {
		if s := strings.TrimSpace(string(r)); s != "" {
			report.Recommendations = append(report.Recommendations, s)
		}
	}

	report.Tally = metricReviewTally{Stored: len(metrics), Kept: len(metrics) - len(removed), Missed: len(report.MissedMetrics)}
	for _, cat := range removed {
		switch cat {
		case "duplicate":
			report.Tally.Duplicate++
		case "formula_input":
			report.Tally.FormulaInput++
		default:
			report.Tally.NotMetric++
		}
	}

	dropped := make([]string, 0, len(droppedSet))
	for id := range droppedSet {
		dropped = append(dropped, id)
	}
	sort.Strings(dropped)
	return report, dropped, nil
}

func normalizeMetricReviewSeverity(s string) string {
	switch v := strings.ToLower(strings.TrimSpace(s)); v {
	case "high", "medium", "low":
		return v
	default:
		return "medium"
	}
}

func normalizeMetricReviewCategory(s string) string {
	switch v := strings.ToLower(strings.TrimSpace(s)); v {
	case "not_metric", "duplicate", "formula_input":
		return v
	default:
		return "not_metric"
	}
}

// formatMetricReviewSpans renders source_line_spans (e.g. ["154","156"]) as
// "154, 156" for the snapshot.
func formatMetricReviewSpans(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var spans []any
	if err := json.Unmarshal(raw, &spans); err != nil {
		return ""
	}
	parts := make([]string, 0, len(spans))
	for _, s := range spans {
		if t := strings.TrimSpace(fmt.Sprint(s)); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, ", ")
}

// metricReviewLangLabel renders a language for the LLM, e.g. "zh-cn (Simplified Chinese)".
func metricReviewLangLabel(lang string) string {
	return fmt.Sprintf("%s (%s)", lang, metricReviewLangNames[lang])
}

type translateMetricReviewRequest struct {
	Lang string `json:"lang"`
}

// TranslateMetricReview handles POST /api/v1/kb/metric-reviews/:record_id/translate
// with body {"lang": "en"|"zh-cn"} (design D4 of metric-review-i18n-export). A
// review already running or done in lang is returned as is; otherwise the newest
// done review in another language is translated in the background into a new row.
func TranslateMetricReview(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_MRV_400")
	defer rc.Close()
	logger := rc.GetLogger()
	ctx := c.Request().Context()
	db := ApiTypes.ProjectDBHandle

	recordID, err := strconv.ParseInt(strings.TrimSpace(c.Param("record_id")), 10, 64)
	if err != nil || recordID <= 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid record_id (CWB_KB_MRV_410)"})
	}
	var req translateMetricReviewRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_KB_MRV_411)"})
	}
	lang, ok := normalizeMetricReviewLang(req.Lang)
	if !ok || strings.TrimSpace(req.Lang) == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "unsupported or missing lang (CWB_KB_MRV_412)"})
	}

	latest, err := loadLatestMetricReview(ctx, db, recordID, lang)
	if err != nil {
		logger.Error("load latest metric review failed", "record_id", recordID, "lang", lang, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load review (CWB_KB_MRV_420)"})
	}
	if latest != nil {
		applyStaleRunningStatus(latest, time.Now())
		if latest.Status != metricReviewStatusFailed {
			logger.Info("review already exists in target language; not translating", "record_id", recordID, "lang", lang, "review_id", latest.ID, "status", latest.Status)
			return c.JSON(http.StatusOK, metricReviewResponse{Status: true, Review: latest})
		}
	}
	source, err := loadTranslationSource(ctx, db, recordID, lang)
	if err != nil {
		logger.Error("load translation source failed", "record_id", recordID, "lang", lang, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load review (CWB_KB_MRV_421)"})
	}
	if source == nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: fmt.Sprintf("record %d has no finished review in another language to translate (CWB_KB_MRV_422)", recordID)})
	}

	var userName, userID string
	if user := rc.IsAuthenticated(); user != nil {
		userName, userID = user.UserName, user.UserId
	}
	sourceID := source.ID
	row := &metricReviewRow{InputRecordID: recordID, Lang: lang, Status: metricReviewStatusRunning,
		MetricsCount: source.MetricsCount, CreatedBy: userName, TranslatedFromID: &sourceID}
	if err := db.QueryRowContext(ctx, `
INSERT INTO kb.metric_reviews (input_record_id, lang, status, metrics_count, created_by, translated_from_id)
VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6)
RETURNING id, created_at`, recordID, lang, metricReviewStatusRunning, source.MetricsCount, userName, sourceID).Scan(&row.ID, &row.CreatedAt); err != nil {
		logger.Error("insert metric review translation failed", "record_id", recordID, "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to create translation (CWB_KB_MRV_430)"})
	}
	logger.Info("starting metric review translation", "record_id", recordID, "lang", lang, "review_id", row.ID, "source_review_id", sourceID, "source_lang", source.Lang)

	go runMetricReviewTranslation(db, row.ID, recordID, source.Report, lang, userID)

	return c.JSON(http.StatusOK, metricReviewResponse{Status: true, Review: row, Started: true})
}

// runMetricReviewTranslation is the background translation run.
func runMetricReviewTranslation(db *sql.DB, reviewID, recordID int64, sourceReport json.RawMessage, lang, userID string) {
	logger := loggerutil.CreateDefaultLogger("20260929-382")
	ctx, cancel := context.WithTimeout(context.Background(), metricReviewRunTimeout)
	defer cancel()

	report, modelName, promptName, err := executeMetricReviewTranslation(ctx, logger, recordID, sourceReport, lang, userID)
	finishMetricReview(db, logger, reviewID, recordID, report, modelName, promptName, err)
}

func executeMetricReviewTranslation(ctx context.Context, logger ApiTypes.JimoLogger, recordID int64, sourceReport json.RawMessage, lang, userID string) (metricReviewReport, string, string, error) {
	var report metricReviewReport
	if err := json.Unmarshal(sourceReport, &report); err != nil {
		return report, "", "", fmt.Errorf("source review report is not valid JSON (CWB_KB_MRV_440): %w", err)
	}
	promptText, promptName, err := loadMetricReviewPrompt("REVIEW_METRICS_TRANSLATE_PROMPT", "CWB_KB_MRV_441", "CWB_KB_MRV_442")
	if err != nil {
		return report, "", promptName, err
	}
	cfg, client, err := newMetricReviewLLMClient(logger)
	if err != nil {
		return report, cfg.ModelName, promptName, err
	}

	strs := collectMetricReviewStrings(&report)
	inputJSON, err := json.Marshal(map[string]any{"target_language": metricReviewLangLabel(lang), "strings": strs})
	if err != nil {
		return report, cfg.ModelName, promptName, fmt.Errorf("marshal translation input failed (CWB_KB_MRV_443): %w", err)
	}
	logger.Info("calling LLM for metric review translation", "record_id", recordID, "lang", lang, "model_name", cfg.ModelName, "strings", len(strs))
	payload, err := client.ExtractJSON(ctx, llmclients.JSONExtractionInput{
		UserID:     userID,
		PromptName: promptName,
		PromptText: promptText,
		ModelName:  cfg.ModelName,
		InputText:  string(inputJSON),
		RecordID:   recordID,
		CallReason: "review_metrics_translate",
		CallLoc:    "MID-20260929-04",
	})
	if err != nil {
		return report, cfg.ModelName, promptName, fmt.Errorf("LLM call failed (CWB_KB_MRV_444): %w", err)
	}
	translated, _ := payload["strings"].(map[string]any)
	applied := applyMetricReviewStrings(&report, translated)
	if applied < len(strs) {
		logger.Warn("translation left some strings untranslated; kept source text", "record_id", recordID, "lang", lang, "applied", applied, "total", len(strs))
	}
	return report, cfg.ModelName, promptName, nil
}

// metricReviewProseFields lists the report fields a translation changes (design
// D3), keyed by path. IDs, severities, categories, field names, line numbers,
// values/units, tally and snapshot are not included and so are copied
// unchanged. Multi-word stored/suggested values are included because the model
// often writes prose there; the translate prompt keeps literal codes as is.
func metricReviewProseFields(r *metricReviewReport) []struct {
	key string
	ptr *string
} {
	var out []struct {
		key string
		ptr *string
	}
	add := func(key string, p *string) {
		out = append(out, struct {
			key string
			ptr *string
		}{key, p})
	}
	// addValue skips single-token field values (codes such as "qualitative",
	// numbers, units, "≥30"), which the model otherwise sometimes translates.
	addValue := func(key string, p *string) {
		if len(strings.Fields(*p)) > 1 {
			add(key, p)
		}
	}
	add("summary", &r.Summary)
	for i := range r.MissedMetrics {
		add(fmt.Sprintf("missed_metrics.%d.name", i), (*string)(&r.MissedMetrics[i].Name))
		add(fmt.Sprintf("missed_metrics.%d.reason", i), (*string)(&r.MissedMetrics[i].Reason))
	}
	for i := range r.NonMetrics {
		add(fmt.Sprintf("non_metrics.%d.reason", i), (*string)(&r.NonMetrics[i].Reason))
	}
	for i := range r.AttributeIssues {
		addValue(fmt.Sprintf("attribute_issues.%d.stored", i), (*string)(&r.AttributeIssues[i].Stored))
		addValue(fmt.Sprintf("attribute_issues.%d.suggested", i), (*string)(&r.AttributeIssues[i].Suggested))
		add(fmt.Sprintf("attribute_issues.%d.reason", i), (*string)(&r.AttributeIssues[i].Reason))
	}
	for i := range r.Recommendations {
		add(fmt.Sprintf("recommendations.%d", i), &r.Recommendations[i])
	}
	return out
}

// collectMetricReviewStrings returns the non-empty prose fields by path.
func collectMetricReviewStrings(r *metricReviewReport) map[string]string {
	out := map[string]string{}
	for _, f := range metricReviewProseFields(r) {
		if strings.TrimSpace(*f.ptr) != "" {
			out[f.key] = *f.ptr
		}
	}
	return out
}

// applyMetricReviewStrings writes translated strings back by path. A missing,
// empty or non-string value keeps the source text. It returns how many fields
// were replaced.
func applyMetricReviewStrings(r *metricReviewReport, translated map[string]any) int {
	applied := 0
	for _, f := range metricReviewProseFields(r) {
		if strings.TrimSpace(*f.ptr) == "" {
			continue
		}
		if s, ok := translated[f.key].(string); ok && strings.TrimSpace(s) != "" {
			*f.ptr = strings.TrimSpace(s)
			applied++
		}
	}
	return applied
}
