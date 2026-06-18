package docprocessing

import (
	"os"
	"path/filepath"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
)

func TestLoadModelConfigFromEnv_LoadsThinkingType(t *testing.T) {
	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[deepseek-v4-flash]
host = "cloud"
model_name = "deepseek-v4-flash"
api_key = "sk-test"
base_url = "https://api.deepseek.com"
timeout_sec = 100
thinking_type = "disabled"
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("STRUCTURE_MODEL_NAME", "deepseek-v4-flash")
	t.Setenv("STRUCTURE_MODELS_FILE", modelsPath)

	_, _, cfg, err := loadModelConfigFromEnv("STRUCTURE_MODEL_NAME", "STRUCTURE_MODELS_FILE")
	if err != nil {
		t.Fatalf("loadModelConfigFromEnv: %v", err)
	}
	if cfg.ThinkingType != "disabled" {
		t.Fatalf("ThinkingType=%q, want disabled", cfg.ThinkingType)
	}
}

func TestApplyStructureModelConfigToExtractor_SetsThinkingType(t *testing.T) {
	client := &llmclients.OpenAIJSONClient{}
	applyStructureModelConfigToExtractor(client, structureModelConfig{
		ModelName:    "deepseek-v4-flash",
		APIKey:       "sk-test",
		BaseURL:      "https://api.deepseek.com",
		TimeoutSec:   42,
		ThinkingType: "disabled",
	})

	if client.ThinkingType != "disabled" {
		t.Fatalf("ThinkingType=%q, want disabled", client.ThinkingType)
	}
}

func TestApplyStructureModelConfigToExtractor_ClearsThinkingTypeWhenUnset(t *testing.T) {
	client := &llmclients.OpenAIJSONClient{
		ThinkingType: "disabled",
	}

	applyStructureModelConfigToExtractor(client, structureModelConfig{
		ModelName:    "deepseek-v4-flash",
		APIKey:       "sk-test",
		BaseURL:      "https://api.openai.com",
		TimeoutSec:   42,
		ThinkingType: "",
	})

	if client.ThinkingType != "" {
		t.Fatalf("ThinkingType=%q, want empty", client.ThinkingType)
	}
}
