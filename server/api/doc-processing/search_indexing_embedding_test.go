package docprocessing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chendingplano/deepdoc/server/api/kbsearch"
)

func TestEmbedRegistryRows_LogsEffectiveTimeoutOnEmbeddingFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(1500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"embedding":[0.1,0.2]}]}`))
	}))
	defer server.Close()

	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[embedding-test]
host = "cloud"
model_name = "text-embedding-3-small"
api_key = "sk-test"
base_url = "` + server.URL + `"
timeout_sec = 1
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EMBEDDING_MODEL_NAME", "embedding-test")
	t.Setenv("MODELS_FILE", modelsPath)

	rows := []kbsearch.RegistryRow{{
		ArtifactType:   "relation",
		ArtifactID:     "177_art_41",
		EmbeddingText:  "relation text",
		SearchDocument: "relation text",
	}}
	logger := &fakeLogger{}

	embedRegistryRows(context.Background(), rows, logger)

	if len(logger.warns) != 1 {
		t.Fatalf("warns len=%d, want 1", len(logger.warns))
	}
	if logger.warns[0].message != "embedding failed; row indexed lexical-only" {
		t.Fatalf("warn message=%q", logger.warns[0].message)
	}
	got, ok := logValue(logger.warns[0].args, "embedding_timeout_sec")
	if !ok {
		t.Fatalf("warning args missing embedding_timeout_sec: %#v", logger.warns[0].args)
	}
	if got != 1 {
		t.Fatalf("embedding_timeout_sec=%v, want 1", got)
	}
}

func TestEmbedRegistryRows_UsesBoundedConcurrency(t *testing.T) {
	var current int32
	var maxSeen int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inFlight := atomic.AddInt32(&current, 1)
		defer atomic.AddInt32(&current, -1)

		for {
			prev := atomic.LoadInt32(&maxSeen)
			if inFlight <= prev || atomic.CompareAndSwapInt32(&maxSeen, prev, inFlight) {
				break
			}
		}

		time.Sleep(75 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"embedding": make([]float64, kbsearch.EmbeddingDim),
			}},
		})
	}))
	defer server.Close()

	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[embedding-test]
host = "cloud"
model_name = "text-embedding-3-small"
api_key = "sk-test"
base_url = "` + server.URL + `"
timeout_sec = 5
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EMBEDDING_MODEL_NAME", "embedding-test")
	t.Setenv("MODELS_FILE", modelsPath)
	t.Setenv("EMBEDDING_MAX_GOROUTINES", "2")
	t.Setenv("EMBEDDING_BATCH_SIZE", "1")

	rows := make([]kbsearch.RegistryRow, 6)
	for i := range rows {
		rows[i] = kbsearch.RegistryRow{
			ArtifactType:   "relation",
			ArtifactID:     strconv.Itoa(i + 1),
			EmbeddingText:  "relation text",
			SearchDocument: "relation text",
		}
	}

	embedRegistryRows(context.Background(), rows, &fakeLogger{})

	if got := atomic.LoadInt32(&maxSeen); got != 2 {
		t.Fatalf("max concurrent requests=%d, want 2", got)
	}
	for i, row := range rows {
		if len(row.Embedding) != kbsearch.EmbeddingDim {
			t.Fatalf("row %d embedding len=%d, want %d", i, len(row.Embedding), kbsearch.EmbeddingDim)
		}
	}
}

func TestEmbedRegistryRows_BatchesMultipleRowsPerRequest(t *testing.T) {
	var requests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)

		var body struct {
			Input any `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		inputs, ok := body.Input.([]any)
		if !ok {
			t.Fatalf("input type=%T, want []any", body.Input)
		}
		embs := make([]map[string]any, 0, len(inputs))
		for range inputs {
			embs = append(embs, map[string]any{
				"embedding": make([]float64, kbsearch.EmbeddingDim),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": embs})
	}))
	defer server.Close()

	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[embedding-test]
host = "cloud"
model_name = "text-embedding-3-small"
api_key = "sk-test"
base_url = "` + server.URL + `"
timeout_sec = 5
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EMBEDDING_MODEL_NAME", "embedding-test")
	t.Setenv("MODELS_FILE", modelsPath)
	t.Setenv("EMBEDDING_MAX_GOROUTINES", "1")
	t.Setenv("EMBEDDING_BATCH_SIZE", "3")

	rows := make([]kbsearch.RegistryRow, 6)
	for i := range rows {
		rows[i] = kbsearch.RegistryRow{
			ArtifactType:   "relation",
			ArtifactID:     strconv.Itoa(i + 1),
			EmbeddingText:  "relation text",
			SearchDocument: "relation text",
		}
	}

	embedRegistryRows(context.Background(), rows, &fakeLogger{})

	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("requests=%d, want 2", got)
	}
	for i, row := range rows {
		if len(row.Embedding) != kbsearch.EmbeddingDim {
			t.Fatalf("row %d embedding len=%d, want %d", i, len(row.Embedding), kbsearch.EmbeddingDim)
		}
	}
}

func TestEmbedRegistryRows_RetriesTransientEmbeddingFailures(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt < 3 {
			time.Sleep(1100 * time.Millisecond)
			w.WriteHeader(http.StatusGatewayTimeout)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"embedding": make([]float64, kbsearch.EmbeddingDim),
			}},
		})
	}))
	defer server.Close()

	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[embedding-test]
host = "cloud"
model_name = "text-embedding-3-small"
api_key = "sk-test"
base_url = "` + server.URL + `"
timeout_sec = 1
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EMBEDDING_MODEL_NAME", "embedding-test")
	t.Setenv("MODELS_FILE", modelsPath)
	t.Setenv("EMBEDDING_MAX_GOROUTINES", "1")
	t.Setenv("EMBEDDING_BATCH_SIZE", "1")

	rows := []kbsearch.RegistryRow{{
		ArtifactType:   "entity",
		ArtifactID:     "177_art_166",
		EmbeddingText:  "entity text",
		SearchDocument: "entity text",
	}}

	embedRegistryRows(context.Background(), rows, &fakeLogger{})

	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("attempts=%d, want 3", got)
	}
	if len(rows[0].Embedding) != kbsearch.EmbeddingDim {
		t.Fatalf("embedding len=%d, want %d", len(rows[0].Embedding), kbsearch.EmbeddingDim)
	}
}

func TestEmbedRegistryRows_DoesNotRetryPermanentEmbeddingFailures(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[embedding-test]
host = "cloud"
model_name = "text-embedding-3-small"
api_key = "sk-test"
base_url = "` + server.URL + `"
timeout_sec = 5
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EMBEDDING_MODEL_NAME", "embedding-test")
	t.Setenv("MODELS_FILE", modelsPath)
	t.Setenv("EMBEDDING_MAX_GOROUTINES", "1")

	rows := []kbsearch.RegistryRow{{
		ArtifactType:   "entity",
		ArtifactID:     "177_art_166",
		EmbeddingText:  "entity text",
		SearchDocument: "entity text",
	}}

	embedRegistryRows(context.Background(), rows, &fakeLogger{})

	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts=%d, want 1", got)
	}
	if len(rows[0].Embedding) != 0 {
		t.Fatalf("embedding len=%d, want 0", len(rows[0].Embedding))
	}
}

func TestEmbedRegistryRows_RetriesStatus520EmbeddingFailures(t *testing.T) {
	var attempts int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := atomic.AddInt32(&attempts, 1)
		if attempt < 3 {
			w.WriteHeader(520)
			_, _ = w.Write([]byte(`error code: 520`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{
				"embedding": make([]float64, kbsearch.EmbeddingDim),
			}},
		})
	}))
	defer server.Close()

	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[embedding-test]
host = "cloud"
model_name = "text-embedding-3-small"
api_key = "sk-test"
base_url = "` + server.URL + `"
timeout_sec = 5
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EMBEDDING_MODEL_NAME", "embedding-test")
	t.Setenv("MODELS_FILE", modelsPath)
	t.Setenv("EMBEDDING_MAX_GOROUTINES", "1")
	t.Setenv("EMBEDDING_BATCH_SIZE", "1")

	rows := []kbsearch.RegistryRow{{
		ArtifactType:   "entity",
		ArtifactID:     "177_art_166",
		EmbeddingText:  "entity text",
		SearchDocument: "entity text",
	}}

	embedRegistryRows(context.Background(), rows, &fakeLogger{})

	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("attempts=%d, want 3", got)
	}
	if len(rows[0].Embedding) != kbsearch.EmbeddingDim {
		t.Fatalf("embedding len=%d, want %d", len(rows[0].Embedding), kbsearch.EmbeddingDim)
	}
}

func TestEmbedRegistryRows_QwenCloudEmbeddingsUseConfiguredDimensionsAndMax10Inputs(t *testing.T) {
	var requests int32
	const configuredDimensions = 1024

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)

		var body struct {
			Model      string `json:"model"`
			Input      []any  `json:"input"`
			Dimensions int    `json:"dimensions"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if body.Model != "text-embedding-v4" {
			t.Fatalf("model=%q, want text-embedding-v4", body.Model)
		}
		if body.Dimensions != configuredDimensions {
			t.Fatalf("dimensions=%d, want %d", body.Dimensions, configuredDimensions)
		}
		if len(body.Input) > 10 {
			t.Fatalf("input len=%d, want <= 10", len(body.Input))
		}

		embs := make([]map[string]any, 0, len(body.Input))
		for range body.Input {
			embs = append(embs, map[string]any{
				"embedding": make([]float64, configuredDimensions),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": embs})
	}))
	defer server.Close()

	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[embedding-test]
host = "cloud"
model_name = "text-embedding-v4"
api_key = "sk-test"
base_url = "` + server.URL + `"
timeout_sec = 5
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EMBEDDING_MODEL_NAME", "embedding-test")
	t.Setenv("EMBEDDING_DIMENSIONS", strconv.Itoa(configuredDimensions))
	t.Setenv("MODELS_FILE", modelsPath)
	t.Setenv("EMBEDDING_MAX_GOROUTINES", "1")
	t.Setenv("EMBEDDING_BATCH_SIZE", "20")

	rows := make([]kbsearch.RegistryRow, 12)
	for i := range rows {
		rows[i] = kbsearch.RegistryRow{
			ArtifactType:   "entity",
			ArtifactID:     strconv.Itoa(i + 1),
			EmbeddingText:  "qwen embedding batch",
			SearchDocument: "qwen embedding batch",
		}
	}

	embedRegistryRows(context.Background(), rows, &fakeLogger{})

	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("requests=%d, want 2", got)
	}
	for i, row := range rows {
		if len(row.Embedding) != configuredDimensions {
			t.Fatalf("row %d embedding len=%d, want %d", i, len(row.Embedding), configuredDimensions)
		}
	}
}

func TestEmbedRegistryRows_QwenCloudEmbeddingsSplitBatchesByEstimatedTokenBudget(t *testing.T) {
	var requests int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)

		var body struct {
			Input any `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request body: %v", err)
		}

		totalRunes := 0
		inputCount := 0
		switch input := body.Input.(type) {
		case string:
			inputCount = 1
			totalRunes += len([]rune(input))
		case []any:
			inputCount = len(input)
			for _, item := range input {
				text, ok := item.(string)
				if !ok {
					t.Fatalf("input item type=%T, want string", item)
				}
				totalRunes += len([]rune(text))
			}
		default:
			t.Fatalf("input type=%T, want string or []any", body.Input)
		}
		if totalRunes > 30000 {
			t.Fatalf("total runes=%d, want <= 30000", totalRunes)
		}

		embs := make([]map[string]any, 0, inputCount)
		for range inputCount {
			embs = append(embs, map[string]any{
				"embedding": make([]float64, kbsearch.EmbeddingDim),
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": embs})
	}))
	defer server.Close()

	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[embedding-test]
host = "cloud"
model_name = "text-embedding-v4"
api_key = "sk-test"
base_url = "` + server.URL + `"
timeout_sec = 5
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EMBEDDING_MODEL_NAME", "embedding-test")
	t.Setenv("MODELS_FILE", modelsPath)
	t.Setenv("EMBEDDING_MAX_GOROUTINES", "1")
	t.Setenv("EMBEDDING_BATCH_SIZE", "8")

	longText := strings.Repeat("x", 6000)
	if len(longText) != 6000 {
		t.Fatalf("longText len=%d, want 6000", len(longText))
	}

	rows := make([]kbsearch.RegistryRow, 6)
	for i := range rows {
		rows[i] = kbsearch.RegistryRow{
			ArtifactType:   "entity",
			ArtifactID:     strconv.Itoa(i + 1),
			EmbeddingText:  longText,
			SearchDocument: longText,
		}
	}

	embedRegistryRows(context.Background(), rows, &fakeLogger{})

	if got := atomic.LoadInt32(&requests); got != 2 {
		t.Fatalf("requests=%d, want 2", got)
	}
}
