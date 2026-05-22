package kbhandler

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

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
	toml "github.com/pelletier/go-toml/v2"
)

type metricJSONExtractor interface {
	ExtractJSON(context.Context, llmclients.JSONExtractionInput) (map[string]any, error)
}

var (
	loadMetricsPromptForExtractFn   = loadMetricsPromptForExtract
	loadExtractMetricsModelConfigFn = loadExtractMetricsModelConfig
	newExtractMetricsClientFn       = defaultNewExtractMetricsClient
)

type extractMetricRequest struct {
	RecordID int64    `json:"record_id"`
	Lines    []string `json:"lines"`
}

type extractMetricResponse struct {
	Status  bool             `json:"status"`
	Metrics []map[string]any `json:"metrics,omitempty"`
	Error   string           `json:"error,omitempty"`
}

type saveExtractedMetricsRequest struct {
	RecordID int64            `json:"record_id"`
	Metrics  []map[string]any `json:"metrics"`
}

type saveExtractedMetricsResponse struct {
	Status   bool   `json:"status"`
	Inserted int64  `json:"inserted,omitempty"`
	Error    string `json:"error,omitempty"`
}

func defaultNewExtractMetricsClient(cfg ApiTypes.LLMModelDef, logger ApiTypes.JimoLogger) (metricJSONExtractor, error) {
	return llmclients.NewOpenAIJSONClientFromConfig(llmclients.OpenAIJSONClientConfig{
		ModelName:    cfg.ModelName,
		APIKey:       cfg.APIKey,
		BaseURL:      cfg.BaseURL,
		TimeoutSec:   cfg.TimeoutSec,
		ThinkingType: cfg.ThinkingType,
	}, logger)
}

// ExtractMetric handles POST /api/v1/kb/metrics/extract
// It reads user-selected lines from the raw line file, composes input with overlap
// context, calls the LLM to extract metrics, and returns the extracted metrics
// for client-side review before persistence.
func ExtractMetric(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_400")
	defer rc.Close()
	logger := rc.GetLogger()

	var req extractMetricRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, extractMetricResponse{
			Status: false,
			Error:  fmt.Sprintf("invalid request body (CWB_KB_M_401): %v", err),
		})
	}
	if req.RecordID <= 0 {
		return c.JSON(http.StatusBadRequest, extractMetricResponse{
			Status: false,
			Error:  "record_id must be a positive integer (CWB_KB_M_402)",
		})
	}
	if len(req.Lines) == 0 {
		return c.JSON(http.StatusBadRequest, extractMetricResponse{
			Status: false,
			Error:  "lines must not be empty (CWB_KB_M_403)",
		})
	}

	db := ApiTypes.ProjectDBHandle

	// Resolve input table and fetch record for result_filename
	inputTable, err := resolveInputTable(db)
	if err != nil {
		logger.Error("resolve kb input table failed", "err", err)
		return c.JSON(http.StatusInternalServerError, extractMetricResponse{
			Status: false,
			Error:  "failed to resolve input table (CWB_KB_M_404)",
		})
	}

	q := fmt.Sprintf(`SELECT i.result_filename FROM %s i WHERE i.id = $1`, inputTable)
	var resultFile sql.NullString
	if err := db.QueryRow(q, req.RecordID).Scan(&resultFile); err != nil {
		if err == sql.ErrNoRows {
			return c.JSON(http.StatusNotFound, extractMetricResponse{
				Status: false,
				Error:  fmt.Sprintf("record %d not found (CWB_KB_M_405)", req.RecordID),
			})
		}
		logger.Error("query result_filename failed", "record_id", req.RecordID, "err", err)
		return c.JSON(http.StatusInternalServerError, extractMetricResponse{
			Status: false,
			Error:  "failed to retrieve record (CWB_KB_M_406)",
		})
	}
	if !resultFile.Valid || strings.TrimSpace(resultFile.String) == "" {
		return c.JSON(http.StatusNotFound, extractMetricResponse{
			Status: false,
			Error:  "result_filename is empty (CWB_KB_M_407)",
		})
	}

	rawPath := rawLinePathFor(resultFile.String)
	rawBytes, err := os.ReadFile(rawPath)
	if err != nil {
		logger.Error("read raw line file failed", "path", rawPath, "err", err)
		return c.JSON(http.StatusNotFound, extractMetricResponse{
			Status: false,
			Error:  fmt.Sprintf("raw line file not found: %s (CWB_KB_M_408)", filepath.Base(rawPath)),
		})
	}

	// Parse the line specs (e.g. ["90", "95-97"]) into a set of wanted line numbers.
	wanted := parseLineSpecs(req.Lines)
	if len(wanted) == 0 {
		return c.JSON(http.StatusBadRequest, extractMetricResponse{
			Status: false,
			Error:  "no valid line numbers in lines (CWB_KB_M_409)",
		})
	}

	// Build overlap set: 5 lines before the first and after the last selected line.
	minLine, maxLine := wanted[0], wanted[len(wanted)-1]
	overlap := make(map[int]bool)
	for i := minLine - 5; i < minLine; i++ {
		if i >= 1 {
			overlap[i] = true
		}
	}
	for i := maxLine + 1; i <= maxLine+5; i++ {
		overlap[i] = true
	}
	for _, n := range wanted {
		delete(overlap, n)
	}

	selectedSet := make(map[int]bool, len(wanted))
	for _, n := range wanted {
		selectedSet[n] = true
	}

	// Apply the blocking process: parse each raw line (dropping font/size/coord),
	// set the flag to 'n' for selected lines and 'o' for overlap lines.
	blockLines := make([]docprocessing.BlockLine, 0, len(wanted)+len(overlap))
	for _, raw := range strings.Split(string(rawBytes), "\n") {
		bl, parseErr := docprocessing.ParseRawLineToBlockLine(strings.TrimRight(raw, "\r"))
		if parseErr != nil {
			continue
		}
		if selectedSet[bl.LineNumber] {
			bl.Flag = "n"
			blockLines = append(blockLines, bl)
		} else if overlap[bl.LineNumber] {
			bl.Flag = "o"
			blockLines = append(blockLines, bl)
		}
	}

	if len(blockLines) == 0 {
		return c.JSON(http.StatusBadRequest, extractMetricResponse{
			Status: false,
			Error:  "specified lines not found in file (CWB_KB_M_411)",
		})
	}

	// Compose the block input string: <flag>\t<line_number>\t<page_number>\t<line_type>\t<content>
	var sb strings.Builder
	for _, bl := range blockLines {
		sb.WriteString(bl.String())
		sb.WriteByte('\n')
	}
	composedInput := sb.String()

	// Load prompt text
	promptText, promptRef, err := loadMetricsPromptForExtractFn(logger)
	if err != nil {
		logger.Error("load metrics prompt failed", "err", err,
			"prompt", promptRef)
		return c.JSON(http.StatusInternalServerError, extractMetricResponse{
			Status: false,
			Error:  "failed to load metrics prompt (CWB_KB_M_412)",
		})
	}

	modelRef, modelCfgPath, modelCfg, err := loadExtractMetricsModelConfigFn()
	if err != nil {
		logger.Error("load extract metrics model config failed", "err", err, "model_ref", modelRef, "models_file", modelCfgPath)
		return c.JSON(http.StatusInternalServerError, extractMetricResponse{
			Status: false,
			Error:  fmt.Sprintf("failed to load extract metrics model config (CWB_KB_M_413): %v", err),
		})
	}
	if strings.TrimSpace(modelRef) == "" {
		logger.Error("missing env var EXTRACT_METRICS_MODEL_NAME")
		return c.JSON(http.StatusInternalServerError, extractMetricResponse{
			Status: false,
			Error:  "(MID_26050801) missing env var EXTRACT_METRICS_MODEL_NAME",
		})
	}

	// Initialize LLM client and extract metrics
	client, err := newExtractMetricsClientFn(modelCfg, logger)
	if err != nil {
		logger.Error("create LLM client failed", "err", err, "model_ref", modelRef, "model_name", modelCfg.ModelName)
		return c.JSON(http.StatusInternalServerError, extractMetricResponse{
			Status: false,
			Error:  "failed to create LLM client (CWB_KB_M_413)",
		})
	}

	logger.Info("start extracting metrics",
		"model_ref", modelRef,
		"model_name", modelCfg.ModelName,
		"prompt_name", promptRef,
		"input", composedInput)

	payload, err := client.ExtractJSON(c.Request().Context(), llmclients.JSONExtractionInput{
		PromptText: promptText,
		ModelName:  modelCfg.ModelName,
		InputText:  composedInput,
	})

	if err != nil {
		logger.Error("LLM extraction failed", "err", err)
		return c.JSON(http.StatusInternalServerError, extractMetricResponse{
			Status: false,
			Error:  fmt.Sprintf("LLM extraction failed (CWB_KB_M_414): %v", err),
		})
	}

	metricsRaw, ok := payload["metrics"].([]any)
	if !ok {
		logger.Error("LLM output missing metrics array")
		return c.JSON(http.StatusInternalServerError, extractMetricResponse{
			Status: false,
			Error:  "LLM output missing metrics array (CWB_KB_M_415)",
		})
	}

	metrics := normalizeExtractedMetrics(metricsRaw)

	logger.Info("extracted metrics ready for review", "record_id", req.RecordID, "count", len(metrics))

	return c.JSON(http.StatusOK, extractMetricResponse{
		Status:  true,
		Metrics: metrics,
	})
}

// SaveExtractedMetrics handles POST /api/v1/kb/metrics/save.
// It persists the reviewed metrics returned from the extract API.
func SaveExtractedMetrics(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_430")
	defer rc.Close()
	logger := rc.GetLogger()

	var req saveExtractedMetricsRequest
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return c.JSON(http.StatusBadRequest, saveExtractedMetricsResponse{
			Status: false,
			Error:  fmt.Sprintf("invalid request body (CWB_KB_M_431): %v", err),
		})
	}
	if req.RecordID <= 0 {
		return c.JSON(http.StatusBadRequest, saveExtractedMetricsResponse{
			Status: false,
			Error:  "record_id must be a positive integer (CWB_KB_M_432)",
		})
	}
	if len(req.Metrics) == 0 {
		return c.JSON(http.StatusBadRequest, saveExtractedMetricsResponse{
			Status: false,
			Error:  "metrics must not be empty (CWB_KB_M_433)",
		})
	}

	inserted, err := saveExtractedMetrics(ApiTypes.ProjectDBHandle, req.RecordID, req.Metrics)
	if err != nil {
		logger.Error("save extracted metrics failed", "record_id", req.RecordID, "err", err)
		return c.JSON(http.StatusInternalServerError, saveExtractedMetricsResponse{
			Status: false,
			Error:  fmt.Sprintf("failed to save metrics (CWB_KB_M_434): %v", err),
		})
	}

	return c.JSON(http.StatusOK, saveExtractedMetricsResponse{
		Status:   true,
		Inserted: inserted,
	})
}

// parseLineSpecs converts a slice of line spec strings like ["90", "95-97"]
// into a sorted, deduplicated slice of int line numbers.
func parseLineSpecs(specs []string) []int {
	seen := make(map[int]bool)
	out := make([]int, 0, len(specs)*2)
	for _, s := range specs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if idx := strings.Index(s, "-"); idx > 0 {
			start, err1 := strconv.Atoi(strings.TrimSpace(s[:idx]))
			end, err2 := strconv.Atoi(strings.TrimSpace(s[idx+1:]))
			if err1 != nil || err2 != nil || start < 1 || end < start {
				continue
			}
			for n := start; n <= end; n++ {
				if !seen[n] {
					seen[n] = true
					out = append(out, n)
				}
			}
		} else {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 {
				continue
			}
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	sort.Ints(out)
	return out
}

// loadMetricsPromptForExtract loads the metrics extraction prompt from env vars,
// trying EXTRACT_METRICS_PROMPT (inline or file path) and PROMPT_FILE_NAME.
func loadMetricsPromptForExtract(logger ApiTypes.JimoLogger) (string, string, error) {
	// Metrics prompt is defined in EXTRACT_METRICS_PROMPT, which identifies a file
	ref := strings.TrimSpace(os.Getenv("EXTRACT_METRICS_PROMPT"))
	if ref != "" {
		// If it looks like a file path and the file exists, read it
		filepath := "prompts/" + ref
		logger.Info("prompt name", "name", filepath)
		if bs, err := os.ReadFile(filepath); err == nil {
			text := strings.TrimSpace(string(bs))
			if text != "" {
				return text, ref, nil
			}
		}
	}

	// This is an error!
	return "", "", fmt.Errorf("(MID_26050802) missing EXTRACT_METRICS_PROMPT")

	/*
		// Fall back to PROMPT_FILE_NAME
		ref = strings.TrimSpace(os.Getenv("PROMPT_FILE_NAME"))
		if ref == "" {
			return "", fmt.Errorf("missing EXTRACT_METRICS_PROMPT")
		}

		candidates := make([]string, 0, 6)
		if filepath.IsAbs(ref) {
			candidates = append(candidates, ref)
		} else {
			candidates = append(candidates, ref)
			if promptDir := strings.TrimSpace(os.Getenv("PROMPT_DIR")); promptDir != "" {
				candidates = append(candidates, filepath.Join(promptDir, ref))
			}
			candidates = append(candidates,
				filepath.Join("server", "cmd", "doc-processor", ref),
				filepath.Join("server", "cmd", "doc-processor", "prompts", ref),
				filepath.Join("python", "extract_metrics", "prompts", ref),
				filepath.Join("prompts", ref),
			)
		}

		var lastErr error
		for _, candidate := range candidates {
			bs, err := os.ReadFile(candidate)
			if err != nil {
				lastErr = err
				continue
			}
			text := strings.TrimSpace(string(bs))
			if text != "" {
				return text, nil
			}
		}
		return "", fmt.Errorf("(CWB_KB_M_417) prompt file not found: %w", lastErr)
	*/
}

// normalizeExtractedMetrics normalizes the LLM output, handling both
// short names (subject, desc) and spec names (metric_subject, metric_desc).
func normalizeExtractedMetrics(items []any) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, ok := item.(map[string]any)
		if !ok {
			continue
		}

		m := map[string]any{
			"metric_name":           stringVal(raw, "metric_name"),
			"metric_name_en":        stringVal(raw, "metric_name_en"),
			"metric_subject":        firstNonEmptyVal(raw, "metric_subject", "subject"),
			"metric_subject_en":     firstNonEmptyVal(raw, "metric_subject_en", "subject_en"),
			"metric_desc":           firstNonEmptyVal(raw, "metric_desc", "desc"),
			"metric_desc_en":        firstNonEmptyVal(raw, "metric_desc_en", "desc_en"),
			"metric_context":        firstNonEmptyVal(raw, "metric_context", "context"),
			"metric_context_en":     firstNonEmptyVal(raw, "metric_context_en", "context_en"),
			"metric_keywords":       anySlice(raw, "metric_keywords", "keywords"),
			"metric_keywords_en":    anySlice(raw, "metric_keywords_en", "keywords_en"),
			"location_type":         stringVal(raw, "location_type"),
			"source_line_spans":     anySlice(raw, "source_line_spans", "line_spans"),
			"metric_unit":           stringVal(raw, "metric_unit", "unit"),
			"metric_unit_en":        stringVal(raw, "metric_unit_en", "unit_en"),
			"metric_value":          stringVal(raw, "metric_value"),
			"value_data_type":       stringVal(raw, "value_data_type"),
			"value_range_type":      stringVal(raw, "value_range_type"),
			"value_class":           stringVal(raw, "value_class"),
			"value_class_en":        stringVal(raw, "value_class_en"),
			"formula_or_definition": stringVal(raw, "formula_or_definition"),
			"threshold_or_target":   stringVal(raw, "threshold_or_target"),
			"measurement_frequency": stringVal(raw, "measurement_frequency"),
			"confidence":            confidenceVal(raw, "confidence"),
			"is_explicit_metric":    boolVal(raw, "is_explicit_metric"),
			"table_name_or_section": stringVal(raw, "table_name_or_section"),
			"reasoning_tags":        anySlice(raw, "reasoning_tags"),
		}
		out = append(out, m)
	}
	return out
}

// saveExtractedMetrics inserts the extracted metrics into kb.metrics with event_id = 'rest-api'.
func saveExtractedMetrics(db *sql.DB, inputRecordID int64, metrics []map[string]any) (int64, error) {
	const ddl = `
	CREATE SCHEMA IF NOT EXISTS kb;
	CREATE TABLE IF NOT EXISTS kb.metrics (
		id BIGSERIAL PRIMARY KEY,
		event_id TEXT,
		input_record_id BIGINT NOT NULL,
		metric_id TEXT,
		metric_name TEXT,
		metric_name_en TEXT,
		source_line_spans JSONB,
		metric_subject TEXT,
		metric_subject_en TEXT,
		metric_desc TEXT,
		metric_desc_en TEXT,
		metric_context TEXT,
		metric_context_en TEXT,
		metric_keywords JSONB,
		metric_keywords_en JSONB,
		model_name TEXT,
		prompt_name TEXT,
		location_type TEXT,
		metric_unit TEXT,
		metric_unit_en TEXT,
		metric_value TEXT,
		value_data_type TEXT,
		value_range_type TEXT,
		value_class TEXT,
		value_class_en TEXT,
		formula_or_definition TEXT,
		threshold_or_target TEXT,
		measurement_frequency TEXT,
		confidence DOUBLE PRECISION,
		is_explicit_metric BOOLEAN,
		table_name_or_section TEXT,
		reasoning_tags JSONB,
		category_paths JSONB,
		category_paths_en JSONB,
		search_document TEXT,
		search_vector TSVECTOR,
		ext_info JSONB,
		created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	);`
	if _, err := db.Exec(ddl); err != nil {
		return 0, err
	}

	var existingCount int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM kb.metrics WHERE input_record_id = $1`, inputRecordID).Scan(&existingCount); err != nil {
		existingCount = 0
	}

	const stmt = `
	INSERT INTO kb.metrics (
		event_id, input_record_id, metric_id, metric_name, metric_name_en, source_line_spans,
		metric_subject, metric_subject_en, metric_desc, metric_desc_en,
		metric_context, metric_context_en, metric_keywords, metric_keywords_en,
		model_name, prompt_name, location_type, metric_unit, metric_unit_en,
		metric_value, value_data_type, value_range_type, value_class, value_class_en,
		formula_or_definition, threshold_or_target, measurement_frequency,
		confidence, is_explicit_metric, table_name_or_section, reasoning_tags,
		category_paths, category_paths_en, ext_info
	) VALUES (
		$1,$2,$3,$4,$5,$6::jsonb,$7,$8,$9,$10,$11,$12,$13::jsonb,$14::jsonb,
		$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31::jsonb,
		$32::jsonb,$33::jsonb,$34::jsonb
	)`

	extInfo, _ := json.Marshal(map[string]any{
		"source":         "rest-api",
		"schema_version": "2",
	})

	var inserted int64
	for i, m := range metrics {
		spansJSON, _ := json.Marshal(m["source_line_spans"])
		keywordsJSON, _ := json.Marshal(m["metric_keywords"])
		reasoningJSON, _ := json.Marshal(m["reasoning_tags"])

		var metricNameEn any
		if v := strings.TrimSpace(anyAsString(m["metric_name_en"])); v != "" {
			metricNameEn = v
		}

		metricID := fmt.Sprintf("%d_%d", inputRecordID, existingCount+int64(i)+1)

		categoryPathsJSON, _ := json.Marshal(m["category_paths"])
		var categoryPathsEnVal any
		if v := m["category_paths_en"]; v != nil {
			bs, _ := json.Marshal(v)
			categoryPathsEnVal = string(bs)
		}

		_, err := db.Exec(stmt,
			"rest-api",
			inputRecordID,
			metricID,
			strings.TrimSpace(anyAsString(m["metric_name"])),
			metricNameEn,
			string(spansJSON),
			strings.TrimSpace(anyAsString(m["metric_subject"])),
			strings.TrimSpace(anyAsString(m["metric_subject_en"])),
			strings.TrimSpace(anyAsString(m["metric_desc"])),
			strings.TrimSpace(anyAsString(m["metric_desc_en"])),
			strings.TrimSpace(anyAsString(m["metric_context"])),
			strings.TrimSpace(anyAsString(m["metric_context_en"])),
			string(keywordsJSON),
			nil, // metric_keywords_en - not set for rest-api
			"",  // model_name
			"",  // prompt_name
			strings.TrimSpace(anyAsString(m["location_type"])),
			strings.TrimSpace(anyAsString(m["metric_unit"])),
			strings.TrimSpace(anyAsString(m["metric_unit_en"])),
			strings.TrimSpace(anyAsString(m["metric_value"])),
			strings.TrimSpace(anyAsString(m["value_data_type"])),
			strings.TrimSpace(anyAsString(m["value_range_type"])),
			strings.TrimSpace(anyAsString(m["value_class"])),
			strings.TrimSpace(anyAsString(m["value_class_en"])),
			strings.TrimSpace(anyAsString(m["formula_or_definition"])),
			strings.TrimSpace(anyAsString(m["threshold_or_target"])),
			strings.TrimSpace(anyAsString(m["measurement_frequency"])),
			confidenceVal(m, "confidence"),
			boolVal(m, "is_explicit_metric"),
			strings.TrimSpace(anyAsString(m["table_name_or_section"])),
			string(reasoningJSON),
			string(categoryPathsJSON),
			categoryPathsEnVal,
			string(extInfo),
		)
		if err != nil {
			return inserted, err
		}
		inserted++
	}
	return inserted, nil
}

// --- helper functions ---

func stringVal(m map[string]any, key string, fallbacks ...string) string {
	keys := append([]string{key}, fallbacks...)
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s := anyAsString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func firstNonEmptyVal(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s := anyAsString(v); s != "" {
				return s
			}
		}
	}
	return ""
}

func anyAsString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		if s, ok := x.(fmt.Stringer); ok {
			return s.String()
		}
		return fmt.Sprintf("%v", x)
	}
}

func anySlice(m map[string]any, key string, fallbacks ...string) []any {
	keys := append([]string{key}, fallbacks...)
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if arr, ok := v.([]any); ok {
				return arr
			}
		}
	}
	return nil
}

func confidenceVal(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case json.Number:
		f, _ := x.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		return 0
	}
}

func boolVal(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true") || x == "1"
	case json.Number:
		n, _ := x.Int64()
		return n != 0
	case float64:
		return x != 0
	default:
		return false
	}
}

func loadExtractMetricsModelConfig() (modelRef string, modelPath string, cfg ApiTypes.LLMModelDef, err error) {
	modelRef = strings.TrimSpace(os.Getenv("EXTRACT_METRICS_MODEL_NAME"))
	if modelRef == "" {
		return "", "", ApiTypes.LLMModelDef{}, nil
	}

	modelPath, err = resolveModelsFilePathForKBHandler("MODEL_CONFIG_FILE")
	if err != nil {
		return modelRef, "", ApiTypes.LLMModelDef{}, err
	}

	raw, err := os.ReadFile(modelPath)
	if err != nil {
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(MID_26050802) read %s failed: %w", modelPath, err)
	}

	parsed := ApiTypes.LLMModelsFile{}
	if err := toml.Unmarshal(raw, &parsed); err != nil {
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(MID_26050803) parse %s failed: %w", modelPath, err)
	}

	modelDef, ok := parsed[modelRef]
	if !ok {
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(MID_26050804) model %q not found in %s", modelRef, modelPath)
	}

	modelDef.ModelName = strings.TrimSpace(modelDef.ModelName)
	modelDef.APIKey = strings.TrimSpace(modelDef.APIKey)
	modelDef.BaseURL = strings.TrimSpace(modelDef.BaseURL)
	modelDef.ThinkingType = strings.TrimSpace(modelDef.ThinkingType)

	switch {
	case modelDef.ModelName == "":
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(MID_26050805) model %q in %s missing model_name", modelRef, modelPath)
	case modelDef.APIKey == "":
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(MID_26050806) model %q in %s missing api_key", modelRef, modelPath)
	case modelDef.BaseURL == "":
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(MID_26050807) model %q in %s missing base_url", modelRef, modelPath)
	case modelDef.TimeoutSec <= 0:
		return modelRef, modelPath, ApiTypes.LLMModelDef{}, fmt.Errorf("(MID_26050808) model %q in %s missing or invalid timeout_sec", modelRef, modelPath)
	}

	return modelRef, modelPath, modelDef, nil
}

func resolveModelsFilePathForKBHandler(modelsFileEnv string) (string, error) {
	if override := strings.TrimSpace(os.Getenv(modelsFileEnv)); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("(MID_26050809) %s %q is invalid: %w", modelsFileEnv, override, err)
		}
		return override, nil
	}
	if override := strings.TrimSpace(os.Getenv("MODELS_FILE")); override != "" {
		if _, err := os.Stat(override); err != nil {
			return "", fmt.Errorf("(MID_26050810) MODELS_FILE %q is invalid: %w", override, err)
		}
		return override, nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("(MID_26050811) getwd failed: %w", err)
	}

	cur := wd
	for {
		candidate := filepath.Join(cur, ".models.toml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	return "", fmt.Errorf(".models.toml not found; set %s or MODELS_FILE, or place .models.toml in the current directory tree", modelsFileEnv)
}
