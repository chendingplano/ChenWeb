package docprocessing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
