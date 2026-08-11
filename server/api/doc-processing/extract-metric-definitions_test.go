package docprocessing

import (
	"os"
	"path/filepath"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestNewMetricDefinitionsProcessor_ConfiguresExtractorFromModelsFile(t *testing.T) {
	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[deepseek-flash-chen]
host = "cloud"
model_name = "deepseek-v4-flash"
api_key = "sk-test-metric-defs"
base_url = "https://api.deepseek.com"
timeout_sec = 120
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EXTRACT_METRIC_DEFINITIONS_MODEL_NAME", "deepseek-flash-chen")
	t.Setenv("MODEL_DEF_FILE", modelsPath)

	client := &llmclients.OpenAIJSONClient{}
	p := NewMetricDefinitionsProcessor(client, nil)

	if p.ModelErr != nil {
		t.Fatalf("ModelErr = %v, want nil", p.ModelErr)
	}
	if client.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("extractor BaseURL = %q, want https://api.deepseek.com (never wired from .models.toml)", client.BaseURL)
	}
	if client.APIKey != "sk-test-metric-defs" {
		t.Fatalf("extractor APIKey = %q, want sk-test-metric-defs (never wired from .models.toml)", client.APIKey)
	}
	if p.ModelName != "deepseek-v4-flash" {
		t.Fatalf("ModelName = %q, want deepseek-v4-flash (the resolved model_name, not the profile ref)", p.ModelName)
	}
}
