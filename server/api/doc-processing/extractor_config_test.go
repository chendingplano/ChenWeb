package docprocessing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractorConfigValidationAndIsolation(t *testing.T) {
	t.Setenv("KB_CONFIG_FILE", filepath.Join(t.TempDir(), "config.toml"))
	for _, tc := range []struct {
		name string
		cfg  ProductsConfig
	}{
		{"metrics-simple", ProductsConfig{"EXTRACT_METRICS_MAX_TASKS": "3"}},
		{"references", ProductsConfig{"EXTRACT_REFERENCES_PROMPT": "prompt-test.md"}},
		{"provisions", ProductsConfig{"EXTRACT_PROVISIONS_INPUT": "blocks", "EXTRACT_PROVISIONS_MAX_TASKS": "2"}},
	} {
		if err := SaveExtractorConfig(tc.name, tc.cfg); err != nil {
			t.Fatal(err)
		}
		got, err := LoadExtractorConfig(tc.name)
		if err != nil || len(got) != len(tc.cfg) {
			t.Fatalf("%s: %#v %v", tc.name, got, err)
		}
		for k, v := range tc.cfg {
			if got[k] != v {
				t.Fatalf("%s: %s = %q", tc.name, k, got[k])
			}
		}
	}
	for _, tc := range []struct {
		name string
		cfg  ProductsConfig
	}{
		{"metrics-simple", ProductsConfig{"EXTRACT_METRICS_MAX_TASKS": "0"}},
		{"references", ProductsConfig{"EXTRACT_PROVISIONS_INPUT": "blocks"}},
		{"provisions", ProductsConfig{"EXTRACT_PROVISIONS_INPUT": "other"}},
		{"../invalid", ProductsConfig{}},
	} {
		if err := SaveExtractorConfig(tc.name, tc.cfg); err == nil {
			t.Fatalf("accepted %s: %#v", tc.name, tc.cfg)
		}
	}
	cfg, err := LoadExtractorConfig("provisions")
	if err != nil || cfg["EXTRACT_PROVISIONS_INPUT"] != "blocks" {
		t.Fatalf("invalid save changed config: %#v %v", cfg, err)
	}
}

func TestExtractorProcessorsApplySavedOverrides(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KB_CONFIG_FILE", filepath.Join(dir, "config.toml"))
	t.Setenv("EXTRACT_METRICS_MAX_TASKS", "2")
	t.Setenv("EXTRACT_PROVISIONS_MAX_TASKS", "2")
	prompt := filepath.Join(dir, "prompt.md")
	if err := os.WriteFile(prompt, []byte("configured prompt"), 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		cfg  ProductsConfig
	}{
		{"metrics-simple", ProductsConfig{"EXTRACT_METRICS_SIMPLE_PROMPT": prompt, "EXTRACT_METRICS_MAX_TASKS": "5"}},
		{"references", ProductsConfig{"EXTRACT_REFERENCES_PROMPT": prompt}},
		{"provisions", ProductsConfig{"EXTRACT_PROVISIONS_PROMPT": prompt, "EXTRACT_PROVISIONS_REVIEW_PROMPT": prompt, "EXTRACT_PROVISIONS_MAX_TASKS": "4", "EXTRACT_PROVISIONS_INPUT": "blocks"}},
	}
	for _, tc := range cases {
		if err := SaveExtractorConfig(tc.name, tc.cfg); err != nil {
			t.Fatal(err)
		}
	}
	m := NewMetricsSimpleProcessor(nil, nil, nil, nil)
	if m.PromptText != "configured prompt" || m.MaxTasks != 5 {
		t.Fatalf("metrics settings: %q %d", m.PromptText, m.MaxTasks)
	}
	r := NewReferencesProcessor(nil, nil, nil, nil)
	if r.PromptText != "configured prompt" {
		t.Fatalf("reference prompt %q", r.PromptText)
	}
	p := NewProvisionsProcessor(nil, nil, nil, nil)
	if p.PromptText != "configured prompt" || p.ReviewPromptText != "configured prompt" || p.ExtractProvisionsMaxTasks != 4 || p.ExtractProvisionsInput != "blocks" {
		t.Fatalf("provision settings: %#v", p)
	}
}
