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

type fakeProvisionExtractor struct {
	out                   map[string]any
	err                   error
	inputText             string
	promptName            string
	calledCount           int
	structuredCalledCount int
	contractNames         []string
}

func (f *fakeProvisionExtractor) ExtractJSON(_ context.Context, in llmclients.JSONExtractionInput) (map[string]any, error) {
	f.calledCount++
	f.inputText = in.InputText
	f.promptName = in.PromptName
	return f.out, f.err
}

func (f *fakeProvisionExtractor) ExtractStructuredJSON(_ context.Context, in llmclients.JSONExtractionInput, contract llmclients.StructuredOutputContract) (*llmclients.StructuredOutputResult, error) {
	f.structuredCalledCount++
	f.contractNames = append(f.contractNames, contract.Name)
	f.inputText = in.InputText
	f.promptName = in.PromptName
	if f.err != nil {
		return nil, f.err
	}
	return &llmclients.StructuredOutputResult{Parsed: f.out}, nil
}

func newExtractProvisionContext(t *testing.T, body string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/provisions/extract", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func TestExtractProvisionUsesStructuredContractWhenAvailable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	tmp := t.TempDir()
	resultPath := filepath.Join(tmp, "ocr_rslt_9.json")
	rawPath := filepath.Join(tmp, "ocr_rslt_9.txt")
	rawBody := "10\t1\tparagraph\tFont\t11\t[0,12,50,22]\tThe device shall log all alarms."
	if err := os.WriteFile(rawPath, []byte(rawBody), 0o644); err != nil {
		t.Fatalf("write raw line file: %v", err)
	}

	expectResolveInputTablePlural(mock)
	resultQuery := regexp.QuoteMeta(`SELECT i.result_filename FROM kb.inputs i WHERE i.id = $1`)
	mock.ExpectQuery(resultQuery).
		WithArgs(int64(9)).
		WillReturnRows(sqlmock.NewRows([]string{"result_filename"}).AddRow(resultPath))

	oldPromptLoader := loadProvisionsPromptForExtractFn
	oldModelLoader := loadExtractProvisionsModelConfigFn
	oldClientFactory := newExtractProvisionsClientFn
	defer func() {
		loadProvisionsPromptForExtractFn = oldPromptLoader
		loadExtractProvisionsModelConfigFn = oldModelLoader
		newExtractProvisionsClientFn = oldClientFactory
	}()

	loadProvisionsPromptForExtractFn = func(_ ApiTypes.JimoLogger) (string, string, error) {
		return "extract provisions", "prompt-extract-provisions.md", nil
	}
	loadExtractProvisionsModelConfigFn = func() (string, string, ApiTypes.LLMModelDef, error) {
		return "gpt-test", filepath.Join(tmp, ".models.toml"), ApiTypes.LLMModelDef{
			ModelName:  "gpt-test",
			APIKey:     "sk-test",
			BaseURL:    "https://api.openai.com",
			TimeoutSec: 60,
		}, nil
	}
	fakeExtractor := &fakeProvisionExtractor{
		out: map[string]any{
			"language": "en",
			"provisions": []any{
				map[string]any{
					"provision": "The device shall log all alarms.",
				},
			},
		},
	}
	newExtractProvisionsClientFn = func(_ string, _ ApiTypes.LLMModelDef, _ ApiTypes.JimoLogger) (provisionJSONExtractor, error) {
		return fakeExtractor, nil
	}

	c, rec := newExtractProvisionContext(t, `{"record_id":9,"source_line_spans":[{"line_number":10,"start_index":0,"end_index":34}]}`)
	if err := ExtractProvisions(c); err != nil {
		t.Fatalf("ExtractProvisions returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload extractProvisionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Status {
		t.Fatalf("expected status=true, body=%s", rec.Body.String())
	}
	if fakeExtractor.structuredCalledCount != 1 {
		t.Fatalf("structuredCalledCount=%d, want 1", fakeExtractor.structuredCalledCount)
	}
	if fakeExtractor.calledCount != 0 {
		t.Fatalf("calledCount=%d, want 0", fakeExtractor.calledCount)
	}
	if len(fakeExtractor.contractNames) != 1 || fakeExtractor.contractNames[0] != "chenweb_provision_handler_extraction" {
		t.Fatalf("contractNames=%v, want [chenweb_provision_handler_extraction]", fakeExtractor.contractNames)
	}
	if fakeExtractor.promptName != "prompt-extract-provisions.md" {
		t.Fatalf("promptName=%q, want prompt-extract-provisions.md", fakeExtractor.promptName)
	}
}
