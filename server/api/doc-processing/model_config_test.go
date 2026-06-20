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
	if cfg.ProfileName != "deepseek-v4-flash" {
		t.Fatalf("ProfileName=%q, want deepseek-v4-flash", cfg.ProfileName)
	}
}

func TestApplyStructureModelConfigToExtractor_SetsThinkingTypeAndProfileName(t *testing.T) {
	client := &llmclients.OpenAIJSONClient{}
	applyStructureModelConfigToExtractor(client, structureModelConfig{
		ProfileName:  "deepseek-v4-flash",
		ModelName:    "deepseek-v4-flash",
		APIKey:       "sk-test",
		BaseURL:      "https://api.deepseek.com",
		TimeoutSec:   42,
		ThinkingType: "disabled",
	})

	if client.ThinkingType != "disabled" {
		t.Fatalf("ThinkingType=%q, want disabled", client.ThinkingType)
	}
	if client.ProfileName != "deepseek-v4-flash" {
		t.Fatalf("ProfileName=%q, want deepseek-v4-flash", client.ProfileName)
	}
}

func TestApplyStructureModelConfigToExtractor_ClearsThinkingTypeAndProfileNameWhenUnset(t *testing.T) {
	client := &llmclients.OpenAIJSONClient{
		ThinkingType: "disabled",
		ProfileName:  "old-profile",
	}

	applyStructureModelConfigToExtractor(client, structureModelConfig{
		ProfileName:  "",
		ModelName:    "deepseek-v4-flash",
		APIKey:       "sk-test",
		BaseURL:      "https://api.openai.com",
		TimeoutSec:   42,
		ThinkingType: "",
	})

	if client.ThinkingType != "" {
		t.Fatalf("ThinkingType=%q, want empty", client.ThinkingType)
	}
	if client.ProfileName != "" {
		t.Fatalf("ProfileName=%q, want empty", client.ProfileName)
	}
}

func TestNewOpenAIJSONClientForStructureModel_PreservesProfileName(t *testing.T) {
	client := newOpenAIJSONClientForStructureModel(structureModelConfig{
		ProfileName: "embedding-test",
		ModelName:   "text-embedding-3-small",
		APIKey:      "sk-test",
		BaseURL:     "https://api.openai.com",
		TimeoutSec:  55,
	}, 1536)
	if client == nil {
		t.Fatalf("client is nil")
	}
	if client.ProfileName != "embedding-test" {
		t.Fatalf("ProfileName=%q, want embedding-test", client.ProfileName)
	}
	if client.EmbeddingDimensions != 1536 {
		t.Fatalf("EmbeddingDimensions=%d, want 1536", client.EmbeddingDimensions)
	}
	if client.HTTPClient == nil || client.HTTPClient.Timeout.Seconds() != 55 {
		t.Fatalf("timeout=%v, want 55s", client.HTTPClient)
	}
}
