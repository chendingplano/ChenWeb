package kbhandler

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func writeTestConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return path
}

func TestLoadKbFrontendConfigReadsDefaultLanguageList(t *testing.T) {
	path := writeTestConfig(t, `
[frontend]
topic_types = ["fact"]
supported_languages = ["en", "zh-cn", "ja"]
default_language = ["zh-cn"]
`)
	t.Setenv("KB_CONFIG_FILE", path)

	cfg, err := LoadKbFrontendConfig()
	if err != nil {
		t.Fatalf("LoadKbFrontendConfig: %v", err)
	}
	if len(cfg.DefaultLanguage) != 1 || cfg.DefaultLanguage[0] != "zh-cn" {
		t.Fatalf("DefaultLanguage = %#v, want [zh-cn]", cfg.DefaultLanguage)
	}
	if len(cfg.SupportedLanguages) != 3 || cfg.SupportedLanguages[2] != "ja" {
		t.Fatalf("SupportedLanguages = %#v", cfg.SupportedLanguages)
	}
}

func TestLoadKbFrontendConfigDefaultsDefaultLanguageToEnWhenAbsent(t *testing.T) {
	path := writeTestConfig(t, `
[frontend]
supported_languages = ["en", "zh-cn"]
`)
	t.Setenv("KB_CONFIG_FILE", path)

	cfg, err := LoadKbFrontendConfig()
	if err != nil {
		t.Fatalf("LoadKbFrontendConfig: %v", err)
	}
	if len(cfg.DefaultLanguage) != 1 || cfg.DefaultLanguage[0] != "en" {
		t.Fatalf("DefaultLanguage = %#v, want [en]", cfg.DefaultLanguage)
	}
}

func TestLoadKbFrontendConfigReadsDefaultProcessors(t *testing.T) {
	path := writeTestConfig(t, `
[doc-processing-packages]
Default = ["extract_metrics"]
Minimal = ["extract_products"]

[doc-processing]
required_processors = ["extract_metrics", "extract_products"]
default_processors = ["extract_products"]
`)
	t.Setenv("KB_CONFIG_FILE", path)

	cfg, err := LoadKbFrontendConfig()
	if err != nil {
		t.Fatalf("LoadKbFrontendConfig: %v", err)
	}
	if len(cfg.DefaultProcessors) != 1 || cfg.DefaultProcessors[0] != "extract_products" {
		t.Fatalf("DefaultProcessors = %#v, want [extract_products]", cfg.DefaultProcessors)
	}
	if got, want := cfg.ProcessorPackages["Default"], []string{"extract_metrics"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ProcessorPackages[Default] = %#v, want %v", got, want)
	}
}

func TestLoadKbFrontendConfigMergesSiblingLocalConfig(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(basePath, []byte("[frontend]\ntopic_types = [\"fact\"]\n"), 0o644); err != nil {
		t.Fatalf("write base config: %v", err)
	}
	localBody := "[doc-processing-packages]\nDefault = [\"extract_metrics\"]\n\n[doc-processing]\nrequired_processors = [\"extract_metrics\", \"extract_products\"]\ndefault_processors = [\"extract_metrics\"]\n"
	if err := os.WriteFile(filepath.Join(dir, "config.local.toml"), []byte(localBody), 0o644); err != nil {
		t.Fatalf("write local config: %v", err)
	}
	t.Setenv("KB_CONFIG_FILE", basePath)

	cfg, err := LoadKbFrontendConfig()
	if err != nil {
		t.Fatalf("LoadKbFrontendConfig: %v", err)
	}
	if got, want := cfg.RequiredProcessors, []string{"extract_metrics", "extract_products"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("RequiredProcessors = %#v, want %v", got, want)
	}
	if got, want := cfg.ProcessorPackages["Default"], []string{"extract_metrics"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ProcessorPackages[Default] = %#v, want %v", got, want)
	}
}
