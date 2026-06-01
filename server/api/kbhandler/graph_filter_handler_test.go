package kbhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/labstack/echo/v4"
)

type fakeGraphFilterEmbedder struct {
	vec []float64
}

func (f fakeGraphFilterEmbedder) Embed(_ context.Context, _ llmclients.EmbedInput) ([]float64, error) {
	return f.vec, nil
}

func TestFilterGraphSemanticMatchesCurrentLevelEmbeddings(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "finance", "metadata.txt"), `"desc":"Finance"`)
	mustWriteFile(t, filepath.Join(root, "finance", "category.embed"), "[1,0]\n")
	mustWriteFile(t, filepath.Join(root, "research", "metadata.txt"), `"desc":"Research"`)
	mustWriteFile(t, filepath.Join(root, "research", "category.embed"), "[0,1]\n")
	mustWriteFile(t, filepath.Join(root, "finance", "tax", "metadata.txt"), `"desc":"Tax"`)
	mustWriteFile(t, filepath.Join(root, "finance", "tax", "category.embed"), "[1,0]\n")

	matches, err := filterGraphSemanticMatches(context.Background(), graphFilterSemanticParams{
		RootDir:          root,
		CandidatePaths:   []string{"finance", "research"},
		QueryText:        "tax strategy",
		Threshold:        0.8,
		EmbeddingModel:   "test-embedding",
		EmbedderOverride: fakeGraphFilterEmbedder{vec: []float64{1, 0}},
	})
	if err != nil {
		t.Fatalf("filterGraphSemanticMatches: %v", err)
	}

	if len(matches) != 1 || matches[0].CategoryPath != "finance" {
		t.Fatalf("unexpected semantic matches: %+v", matches)
	}
	if matches[0].Score < 0.99 {
		t.Fatalf("expected cosine score near 1.0, got %.4f", matches[0].Score)
	}
}

func TestFilterGraphNodesRejectsThresholdOutsideUnitInterval(t *testing.T) {
	e := echo.New()
	body, _ := json.Marshal(filterGraphNodesRequest{
		Mode:           "topic",
		CandidatePaths: []string{"finance"},
		SemanticText:   "finance",
		Threshold:      1.2,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/graph-node-filter", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := FilterGraphNodes(c); err != nil {
		t.Fatalf("FilterGraphNodes returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestFilterGraphNodesUsesModelConfigAndCategoryEmbedFiles(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "finance", "category.embed"), "[1,0]\n")
	mustWriteFile(t, filepath.Join(root, "research", "category.embed"), "[0,1]\n")
	embeddingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("unexpected embedding path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[{"embedding":[1,0]}]}`)
	}))
	defer embeddingServer.Close()

	modelsPath := filepath.Join(t.TempDir(), ".models.toml")
	mustWriteFile(t, modelsPath, fmt.Sprintf(`
[topic-embedding]
model_name = "text-embedding-3-small"
api_key = "test-key"
base_url = %q
timeout_sec = 5
`, embeddingServer.URL))
	t.Setenv("ARTIFACT_WEB_DIR", root)
	t.Setenv("EMBEDDING_MODEL_NAME", "topic-embedding")
	t.Setenv("MODELS_FILE", modelsPath)

	e := echo.New()
	body, _ := json.Marshal(filterGraphNodesRequest{
		Mode:           "topic",
		CandidatePaths: []string{"finance", "research"},
		SemanticText:   "tax strategy",
		Threshold:      0.6,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/graph-node-filter", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := FilterGraphNodes(c); err != nil {
		t.Fatalf("FilterGraphNodes returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload filterGraphNodesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Matches) != 1 || payload.Matches[0].CategoryPath != "finance" {
		t.Fatalf("unexpected matches: %+v", payload.Matches)
	}
}

func TestNewGraphFilterEmbedderResolvesEmbeddingModelConfig(t *testing.T) {
	modelsPath := filepath.Join(t.TempDir(), ".models.toml")
	mustWriteFile(t, modelsPath, `
[topic-embedding]
model_name = "text-embedding-3-small"
api_key = "test-key"
base_url = "https://example.test/v1"
timeout_sec = 12
`)
	t.Setenv("EMBEDDING_MODEL_NAME", "topic-embedding")
	t.Setenv("MODELS_FILE", modelsPath)

	embedderCfg := newGraphFilterEmbedder("topic")

	if embedderCfg.Err != nil {
		t.Fatalf("newGraphFilterEmbedder returned error: %v", embedderCfg.Err)
	}
	if embedderCfg.ModelName != "text-embedding-3-small" {
		t.Fatalf("modelName=%q, want resolved model_name", embedderCfg.ModelName)
	}
	client, ok := embedderCfg.Embedder.(*llmclients.OpenAIJSONClient)
	if !ok {
		t.Fatalf("embedder type=%T, want *OpenAIJSONClient", embedderCfg.Embedder)
	}
	if client.APIKey != "test-key" || client.BaseURL != "https://example.test/v1" {
		t.Fatalf("unexpected client config: api=%q base=%q", client.APIKey, client.BaseURL)
	}
}

func TestNewGraphFilterEmbedderReportsRequiredModeEnv(t *testing.T) {
	t.Setenv("MODELS_FILE", filepath.Join(t.TempDir(), ".models.toml"))

	embedderCfg := newGraphFilterEmbedder("summary")

	if embedderCfg.Err == nil || !strings.Contains(embedderCfg.Err.Error(), "EMBEDDING_MODEL_NAME") {
		t.Fatalf("expected missing EMBEDDING_MODEL_NAME error, got %+v", embedderCfg.Err)
	}
}
