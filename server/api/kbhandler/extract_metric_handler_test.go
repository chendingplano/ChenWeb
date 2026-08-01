package kbhandler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
)

type fakeMetricExtractor struct {
	out                   map[string]any
	err                   error
	inputText             string
	promptName            string
	calledCount           int
	structuredCalledCount int
	contractNames         []string
}

func (f *fakeMetricExtractor) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	f.calledCount++
	f.inputText = in.InputText
	f.promptName = in.PromptName
	return f.out, f.err
}

func (f *fakeMetricExtractor) ExtractStructuredJSON(_ context.Context, in llmclients.JSONExtractionInput, contract llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error) {
	f.structuredCalledCount++
	f.contractNames = append(f.contractNames, contract.Name)
	f.inputText = in.InputText
	f.promptName = in.PromptName
	if f.err != nil {
		return nil, f.err
	}
	return &llmclients.StructuredOutputResult{Parsed: f.out}, nil
}

func newExtractMetricContext(t *testing.T, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/metrics/extract", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func newSaveExtractedMetricsContext(t *testing.T, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/metrics/save", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestLoadExtractMetricsModelConfig(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), ".models.toml")
	content := `
[deepseek-v4-flash]
host = "cloud"
model_name = "deepseek-v4-flash"
api_key = "sk-test"
base_url = "https://api.openai.com"
timeout_sec = 120
thinking_type = "enabled"
`
	if err := os.WriteFile(modelsPath, []byte(strings.TrimSpace(content)), 0o644); err != nil {
		t.Fatalf("write .models.toml: %v", err)
	}

	t.Setenv("EXTRACT_METRICS_MODEL_NAME", "deepseek-v4-flash")
	t.Setenv("MODELS_FILE", modelsPath)

	modelRef, modelPath, cfg, err := loadExtractMetricsModelConfig()
	if err != nil {
		t.Fatalf("loadExtractMetricsModelConfig error: %v", err)
	}
	if modelRef != "deepseek-v4-flash" {
		t.Fatalf("modelRef=%q", modelRef)
	}
	if modelPath != modelsPath {
		t.Fatalf("modelPath=%q, want %q", modelPath, modelsPath)
	}
	if cfg.ModelName != "deepseek-v4-flash" {
		t.Fatalf("cfg.ModelName=%q", cfg.ModelName)
	}
	if cfg.APIKey != "sk-test" {
		t.Fatalf("cfg.APIKey=%q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://api.openai.com" {
		t.Fatalf("cfg.BaseURL=%q", cfg.BaseURL)
	}
	if cfg.TimeoutSec != 120 {
		t.Fatalf("cfg.TimeoutSec=%d", cfg.TimeoutSec)
	}
	if cfg.ThinkingType != "enabled" {
		t.Fatalf("cfg.ThinkingType=%q", cfg.ThinkingType)
	}
}

func TestLoadExtractMetricsModelConfig_RequiresExplicitAttributes(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), ".models.toml")
	content := `
[deepseek-v4-flash]
host = "cloud"
model_name = "deepseek-v4-flash"
api_key = ""
base_url = "https://api.openai.com"
timeout_sec = 120
`
	if err := os.WriteFile(modelsPath, []byte(strings.TrimSpace(content)), 0o644); err != nil {
		t.Fatalf("write .models.toml: %v", err)
	}

	t.Setenv("EXTRACT_METRICS_MODEL_NAME", "deepseek-v4-flash")
	t.Setenv("MODELS_FILE", modelsPath)

	_, _, _, err := loadExtractMetricsModelConfig()
	if err == nil {
		t.Fatalf("expected error for missing api_key")
	}
	if !strings.Contains(err.Error(), "missing api_key") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractMetricReturnsMetricsWithoutSaving(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	tmp := t.TempDir()
	resultPath := filepath.Join(tmp, "ocr_rslt_7.json")
	rawPath := filepath.Join(tmp, "ocr_rslt_7.txt")
	rawBody := strings.Join([]string{
		"5\t1\theading\tFont\t11\t[0,0,50,10]\tScope",
		"10\t1\tparagraph\tFont\t11\t[0,12,50,22]\tEnergy intensity shall be 12 kWh/m2.",
		"11\t1\tparagraph\tFont\t11\t[0,24,50,34]\tTarget is monthly reporting.",
		"16\t1\tparagraph\tFont\t11\t[0,36,50,46]\tTrailing context",
	}, "\n")
	if err := os.WriteFile(rawPath, []byte(rawBody), 0o644); err != nil {
		t.Fatalf("write raw line file: %v", err)
	}

	expectResolveInputTablePlural(mock)
	resultQuery := regexp.QuoteMeta(`SELECT i.result_filename FROM kb.inputs i WHERE i.id = $1`)
	mock.ExpectQuery(resultQuery).
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"result_filename"}).AddRow(resultPath))

	oldPromptLoader := loadMetricsPromptForExtractFn
	oldModelLoader := loadExtractMetricsModelConfigFn
	oldClientFactory := newExtractMetricsClientFn
	defer func() {
		loadMetricsPromptForExtractFn = oldPromptLoader
		loadExtractMetricsModelConfigFn = oldModelLoader
		newExtractMetricsClientFn = oldClientFactory
	}()

	loadMetricsPromptForExtractFn = func(_ ApiTypes.JimoLogger) (string, string, error) {
		return "extract metrics", "prompt_extract_metrics_v1.txt", nil
	}
	loadExtractMetricsModelConfigFn = func() (string, string, ApiTypes.LLMModelDef, error) {
		return "gpt-test", filepath.Join(tmp, ".models.toml"), ApiTypes.LLMModelDef{
			ModelName:  "gpt-test",
			APIKey:     "sk-test",
			BaseURL:    "https://api.openai.com",
			TimeoutSec: 60,
		}, nil
	}
	fakeExtractor := &fakeMetricExtractor{
		out: map[string]any{
			"metrics": []any{
				map[string]any{
					"metric_name":        "Energy intensity",
					"metric_desc":        "Energy intensity threshold",
					"line_spans":         []any{"10:11"},
					"metric_unit":        "kWh/m2",
					"confidence":         0.91,
					"is_explicit_metric": true,
				},
			},
		},
	}
	newExtractMetricsClientFn = func(_ string, _ ApiTypes.LLMModelDef, _ ApiTypes.JimoLogger) (metricJSONExtractor, error) {
		return fakeExtractor, nil
	}

	c, rec := newExtractMetricContext(t, `{"record_id":7,"lines":["10-11"]}`)
	if err := ExtractMetric(c); err != nil {
		t.Fatalf("ExtractMetric returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload extractMetricResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true, body=%s", rec.Body.String())
	}
	if len(payload.Metrics) != 1 {
		t.Fatalf("metrics len=%d, want 1", len(payload.Metrics))
	}
	if got := strings.TrimSpace(anyAsString(payload.Metrics[0]["metric_name"])); got != "Energy intensity" {
		t.Fatalf("metric_name=%q", got)
	}
	spans, ok := payload.Metrics[0]["source_line_spans"].([]any)
	if !ok || len(spans) != 1 || anyAsString(spans[0]) != "10:11" {
		t.Fatalf("source_line_spans=%#v", payload.Metrics[0]["source_line_spans"])
	}
	if strings.TrimSpace(fakeExtractor.inputText) == "" {
		t.Fatalf("expected extractor input text to be populated")
	}
	if fakeExtractor.promptName != "prompt_extract_metrics_v1.txt" {
		t.Fatalf("promptName=%q, want prompt_extract_metrics_v1.txt", fakeExtractor.promptName)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestExtractMetricUsesStructuredContractWhenAvailable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	tmp := t.TempDir()
	resultPath := filepath.Join(tmp, "ocr_rslt_8.json")
	rawPath := filepath.Join(tmp, "ocr_rslt_8.txt")
	rawBody := "10\t1\tparagraph\tFont\t11\t[0,12,50,22]\tEnergy intensity shall be 12 kWh/m2."
	if err := os.WriteFile(rawPath, []byte(rawBody), 0o644); err != nil {
		t.Fatalf("write raw line file: %v", err)
	}

	expectResolveInputTablePlural(mock)
	resultQuery := regexp.QuoteMeta(`SELECT i.result_filename FROM kb.inputs i WHERE i.id = $1`)
	mock.ExpectQuery(resultQuery).
		WithArgs(int64(8)).
		WillReturnRows(sqlmock.NewRows([]string{"result_filename"}).AddRow(resultPath))

	oldPromptLoader := loadMetricsPromptForExtractFn
	oldModelLoader := loadExtractMetricsModelConfigFn
	oldClientFactory := newExtractMetricsClientFn
	defer func() {
		loadMetricsPromptForExtractFn = oldPromptLoader
		loadExtractMetricsModelConfigFn = oldModelLoader
		newExtractMetricsClientFn = oldClientFactory
	}()

	loadMetricsPromptForExtractFn = func(_ ApiTypes.JimoLogger) (string, string, error) {
		return "extract metrics", "prompt_extract_metrics_v1.txt", nil
	}
	loadExtractMetricsModelConfigFn = func() (string, string, ApiTypes.LLMModelDef, error) {
		return "gpt-test", filepath.Join(tmp, ".models.toml"), ApiTypes.LLMModelDef{
			ModelName:  "gpt-test",
			APIKey:     "sk-test",
			BaseURL:    "https://api.openai.com",
			TimeoutSec: 60,
		}, nil
	}
	fakeExtractor := &fakeMetricExtractor{
		out: map[string]any{
			"metrics": []any{
				map[string]any{
					"metric_name": "Energy intensity",
				},
			},
		},
	}
	newExtractMetricsClientFn = func(_ string, _ ApiTypes.LLMModelDef, _ ApiTypes.JimoLogger) (metricJSONExtractor, error) {
		return fakeExtractor, nil
	}

	c, rec := newExtractMetricContext(t, `{"record_id":8,"lines":["10"]}`)
	if err := ExtractMetric(c); err != nil {
		t.Fatalf("ExtractMetric returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if fakeExtractor.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", fakeExtractor.structuredCalledCount)
	}
	if fakeExtractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", fakeExtractor.calledCount)
	}
	if len(fakeExtractor.contractNames) != 1 || fakeExtractor.contractNames[0] != "chenweb_metric_handler_extraction" {
		t.Fatalf("contractNames=%v, want [chenweb_metric_handler_extraction]", fakeExtractor.contractNames)
	}
	if fakeExtractor.promptName != "prompt_extract_metrics_v1.txt" {
		t.Fatalf("promptName=%q, want prompt_extract_metrics_v1.txt", fakeExtractor.promptName)
	}
}

func TestSaveExtractedMetricsPersistsProvidedMetrics(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS kb;").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM kb.metrics WHERE input_record_id").
		WithArgs(int64(7)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectExec("INSERT INTO kb.metrics").
		WithArgs(
			"rest-api",
			int64(7),
			"7_mtc_1",
			"Energy intensity",
			nil,
			`["10:11"]`,
			"Building envelope",
			"",
			"Energy intensity threshold",
			"",
			"",
			"",
			`["energy","intensity"]`,
			nil,
			"",
			"",
			"table_row",
			"kWh/m2",
			"",
			"12",
			"number",
			"exact",
			"performance",
			"",
			nil,
			nil,
			"",
			"",
			"",
			"monthly",
			0.91,
			true,
			"Table 2",
			`["named_metric"]`,
			`["energy_eff"]`,
			`["energy_efficiency"]`,
			sqlmock.AnyArg(),
			nil,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(101, 1))

	c, rec := newSaveExtractedMetricsContext(t, `{
		"record_id": 7,
		"metrics": [
			{
				"metric_name": "Energy intensity",
				"metric_subject": "Building envelope",
				"metric_desc": "Energy intensity threshold",
				"source_line_spans": ["10:11"],
				"metric_keywords": ["energy", "intensity"],
				"location_type": "table_row",
				"metric_unit": "kWh/m2",
				"metric_value": "12",
				"value_data_type": "number",
				"value_range_type": "exact",
				"value_class": "performance",
				"measurement_frequency": "monthly",
				"confidence": 0.91,
				"is_explicit_metric": true,
				"table_name_or_section": "Table 2",
				"reasoning_tags": ["named_metric"],
				"metric_categories": ["energy_eff"],
				"metric_categories_en": ["energy_efficiency"]
			}
		]
	}`)
	if err := SaveExtractedMetrics(c); err != nil {
		t.Fatalf("SaveExtractedMetrics returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Status   bool  `json:"status"`
		Inserted int64 `json:"inserted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true")
	}
	if payload.Inserted != 1 {
		t.Fatalf("inserted=%d, want 1", payload.Inserted)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

// TestSaveExtractedMetricsPersistsRangeValueMinMaxCondition locks in the v5
// extraction path: a range metric carries numeric value_min/value_max and a
// condition string, and the handler persists all three to kb.metrics.
func TestSaveExtractedMetricsPersistsRangeValueMinMaxCondition(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	mock.ExpectExec("CREATE SCHEMA IF NOT EXISTS kb;").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM kb.metrics WHERE input_record_id").
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))
	mock.ExpectExec("INSERT INTO kb.metrics").
		WithArgs(
			"rest-api",
			int64(9),
			"9_mtc_1",
			"Contrast ratio",
			nil,
			`["5"]`,
			"Display",
			"",
			"",
			"",
			"",
			"",
			`["contrast"]`,
			nil,
			"",
			"",
			"table_row",
			"ratio",
			"",
			"500:1 至 2000:1",
			"number",
			"range",
			"requirement",
			"",
			500.0,
			2000.0,
			"常温下",
			"",
			"",
			"",
			0.9,
			true,
			"Table 1",
			`["range_metric"]`,
			`["ratio"]`,
			`["contrast_ratio"]`,
			sqlmock.AnyArg(),
			nil,
			sqlmock.AnyArg(),
		).
		WillReturnResult(sqlmock.NewResult(102, 1))

	c, rec := newSaveExtractedMetricsContext(t, `{
		"record_id": 9,
		"metrics": [
			{
				"metric_name": "Contrast ratio",
				"metric_subject": "Display",
				"source_line_spans": ["5"],
				"metric_keywords": ["contrast"],
				"location_type": "table_row",
				"metric_unit": "ratio",
				"metric_value": "500:1 至 2000:1",
				"value_data_type": "number",
				"value_range_type": "range",
				"value_class": "requirement",
				"value_min": 500,
				"value_max": 2000,
				"condition": "常温下",
				"measurement_frequency": "",
				"confidence": 0.9,
				"is_explicit_metric": true,
				"table_name_or_section": "Table 1",
				"reasoning_tags": ["range_metric"],
				"metric_categories": ["ratio"],
				"metric_categories_en": ["contrast_ratio"]
			}
		]
	}`)
	if err := SaveExtractedMetrics(c); err != nil {
		t.Fatalf("SaveExtractedMetrics returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}
