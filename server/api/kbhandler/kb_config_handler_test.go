package kbhandler

import (
	"os"
	"path/filepath"
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
