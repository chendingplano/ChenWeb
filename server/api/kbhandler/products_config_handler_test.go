package kbhandler

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProductConfigModelsOverlay(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(base, []byte("[doc-processing]\ndoc_processing_models = [\"base\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte("[doc-processing]\ndoc_processing_models = [\"first\", \"second\"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := productConfigModels(base)
	if err != nil || !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("models = %v, %v", got, err)
	}
}
func TestProductPromptChoices(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PROMPT_DIR", "")
	if err := os.Mkdir(filepath.Join(dir, "prompts"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"prompt-extract-product-mentions-v2.md", "prompt-extract-product-mentions-v1.md", "prompt-translate-products-v2.md", "unrelated.md"} {
		if err := os.WriteFile(filepath.Join(dir, "prompts", name), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got := productPromptChoices(dir)
	if !reflect.DeepEqual(got["EXTRACT_PRODUCT_MENTIONS_PROMPT"], []string{"prompt-extract-product-mentions-v1.md", "prompt-extract-product-mentions-v2.md"}) {
		t.Fatalf("choices: %v", got)
	}
	if len(got["TRANSLATE_PRODUCTS_PROMPT"]) != 1 || len(got["ENRICH_PRODUCT_MENTION_PROMPT"]) != 0 {
		t.Fatalf("choices: %v", got)
	}
}
