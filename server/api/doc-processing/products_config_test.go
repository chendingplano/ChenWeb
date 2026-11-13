package docprocessing

import (
	"path/filepath"
	"testing"
)

func TestProductsConfigRoundTripAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	want := ProductsConfig{"EXTRACT_PRODUCTS_MAX_TASKS": "3", "EXTRACT_PRODUCTS_REASONING": "false", "EXTRACT_PRODUCT_MENTIONS_MODEL_NAME": "profile", "TRANSLATION_MODEL_NAME": ""}
	if err := SaveProductsConfig(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadProductsConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["EXTRACT_PRODUCTS_MAX_TASKS"] != "3" || got["EXTRACT_PRODUCTS_REASONING"] != "false" {
		t.Fatalf("unexpected config: %#v", got)
	}
	if _, ok := got["TRANSLATION_MODEL_NAME"]; ok {
		t.Fatal("empty value should inherit")
	}
	for _, invalid := range []ProductsConfig{{"EXTRACT_PRODUCTS_MAX_TASKS": "0"}, {"EXTRACT_PRODUCTS_REASONING": "maybe"}, {"UNKNOWN": "x"}, {"EXTRACT_PRODUCT_TRANSLATION": "true"}} {
		if err := SaveProductsConfig(path, invalid); err == nil {
			t.Fatalf("accepted %#v", invalid)
		}
	}
	got, err = LoadProductsConfig(path)
	if err != nil || got["EXTRACT_PRODUCTS_MAX_TASKS"] != "3" {
		t.Fatalf("invalid save changed existing settings: %#v, %v", got, err)
	}
}
func TestProductsConfigOverridesEnvironment(t *testing.T) {
	t.Setenv("EXTRACT_PRODUCTS_MAX_TASKS", "2")
	t.Setenv("EXTRACT_PRODUCTS_REASONING", "true")
	c := ProductsConfig{"EXTRACT_PRODUCTS_MAX_TASKS": "4", "EXTRACT_PRODUCTS_REASONING": "false"}
	if c.integer("EXTRACT_PRODUCTS_MAX_TASKS", 1) != 4 || c.boolean("EXTRACT_PRODUCTS_REASONING", true) {
		t.Fatal("overrides ignored")
	}
	if (ProductsConfig{}).integer("EXTRACT_PRODUCTS_MAX_TASKS", 1) != 2 {
		t.Fatal("environment fallback ignored")
	}
}

func TestProductsProcessorUsesSavedConfiguration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KB_CONFIG_FILE", filepath.Join(dir, "config.toml"))
	t.Setenv("EXTRACT_PRODUCTS_MAX_TASKS", "2")
	t.Setenv("EXTRACT_PRODUCTS_REASONING", "true")
	if err := SaveProductsConfig(ProductsConfigPath(), ProductsConfig{"EXTRACT_PRODUCTS_MAX_TASKS": "5", "EXTRACT_PRODUCTS_REASONING": "false", "EXTRACT_PRODUCTS_MERGED_PASS": "true"}); err != nil {
		t.Fatal(err)
	}
	p := NewProductsProcessor(nil, nil, nil, nil)
	if p.MaxTasks != 5 || p.Reasoning || !p.MergedPass {
		t.Fatalf("saved settings not applied: tasks=%d reasoning=%v merged=%v", p.MaxTasks, p.Reasoning, p.MergedPass)
	}
}
