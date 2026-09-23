package docprocessing

import (
	"os"
	"path/filepath"
	"testing"
)

// Regression test for the Spend Reports dashboard showing a phantom, always-empty
// "deepseek-flash-processor" chart: when EXTRACT_TOPIC_MODEL_NAME names a profile
// that is not defined in the resolved models file, the loader must fail loudly
// instead of silently treating the raw profile key as a literal model name. The
// silent fallback sent invalid `model` values straight to the provider (which
// rejected them) and permanently polluted llm_usage_event.model_name.
func TestLoadFixedSizeTopicModelFromEnv_UnknownProfile_Errors(t *testing.T) {
	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[deepseek-v4-flash]
host = "cloud"
model_name = "deepseek-v4-flash"
api_key = "sk-test"
base_url = "https://api.deepseek.com"
timeout_sec = 100
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("EXTRACT_TOPIC_MODEL_NAME", "deepseek-flash-processor")
	t.Setenv("CHUNK_EXTRACT_TOPIC_MODELS_FILE", modelsPath)

	_, _, cfg, err := loadFixedSizeTopicModelFromEnv()
	if err == nil {
		t.Fatalf("expected error for unknown profile, got cfg=%+v", cfg)
	}
	if cfg.ModelName == "deepseek-flash-processor" {
		t.Fatalf("must not fall back to the raw profile ref as the model name, got ModelName=%q", cfg.ModelName)
	}
}

func TestLoadFixedSizeSummaryModelFromEnv_UnknownProfile_Errors(t *testing.T) {
	tmp := t.TempDir()
	modelsPath := filepath.Join(tmp, ".models.toml")
	modelsBody := `
[deepseek-v4-flash]
host = "cloud"
model_name = "deepseek-v4-flash"
api_key = "sk-test"
base_url = "https://api.deepseek.com"
timeout_sec = 100
`
	if err := os.WriteFile(modelsPath, []byte(modelsBody), 0o644); err != nil {
		t.Fatalf("write models file: %v", err)
	}

	t.Setenv("CHUNK_SUMMARY_MODEL_NAME", "deepseek-flash-processor")
	t.Setenv("CHUNK_SUMMARY_MODELS_FILE", modelsPath)

	_, _, cfg, err := loadFixedSizeSummaryModelFromEnv()
	if err == nil {
		t.Fatalf("expected error for unknown profile, got cfg=%+v", cfg)
	}
	if cfg.ModelName == "deepseek-flash-processor" {
		t.Fatalf("must not fall back to the raw profile ref as the model name, got ModelName=%q", cfg.ModelName)
	}
}

// resolveModelsFilePath failing (e.g. a stale/invalid override path) must also
// surface as an error rather than silently using the ref as the model name.
func TestLoadFixedSizeTopicModelFromEnv_InvalidModelsFile_Errors(t *testing.T) {
	t.Setenv("EXTRACT_TOPIC_MODEL_NAME", "deepseek-flash-processor")
	t.Setenv("CHUNK_EXTRACT_TOPIC_MODELS_FILE", filepath.Join(t.TempDir(), "does-not-exist.toml"))

	_, _, cfg, err := loadFixedSizeTopicModelFromEnv()
	if err == nil {
		t.Fatalf("expected error for invalid models file, got cfg=%+v", cfg)
	}
	if cfg.ModelName == "deepseek-flash-processor" {
		t.Fatalf("must not fall back to the raw profile ref as the model name, got ModelName=%q", cfg.ModelName)
	}
}
