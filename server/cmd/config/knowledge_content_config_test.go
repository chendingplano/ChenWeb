package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// Verifies that GetKnowledgeContentConfig returns an empty map when no
// [knowledge-content] section is present, so the Knowledge page defaults to
// fully enabled.
func TestGetKnowledgeContentConfigDefaultsToEmpty(t *testing.T) {
	oldConfig := AppConfig
	oldViper := appConfigViper
	t.Cleanup(func() {
		AppConfig = oldConfig
		appConfigViper = oldViper
		viper.Reset()
	})
	viper.Reset()
	appConfigViper = viper.New()
	viper.SetConfigType("toml")

	const sample = `
[frontend]
default_knowledge_store = "Research"
`
	if err := viper.ReadConfig(strings.NewReader(sample)); err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := viper.Unmarshal(&AppConfig); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	appConfigViper = viper.GetViper()

	cfg := GetKnowledgeContentConfig()
	if len(cfg) != 0 {
		t.Fatalf("expected no configured content ids, got %v", cfg)
	}
}

// Verifies that a [knowledge-content] TOML section unmarshals id->bool entries
// into the KnowledgeContent map and is returned by GetKnowledgeContentConfig.
func TestGetKnowledgeContentConfigUnmarshalsIdBooleans(t *testing.T) {
	oldConfig := AppConfig
	oldViper := appConfigViper
	t.Cleanup(func() {
		AppConfig = oldConfig
		appConfigViper = oldViper
		viper.Reset()
	})
	viper.Reset()
	appConfigViper = viper.New()
	viper.SetConfigType("toml")

	const sample = `
[knowledge-content]
kb-doc-wiki = true
kb-metrics = false
`
	if err := viper.ReadConfig(strings.NewReader(sample)); err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := viper.Unmarshal(&AppConfig); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	appConfigViper = viper.GetViper()

	cfg := GetKnowledgeContentConfig()
	if len(cfg) != 2 {
		t.Fatalf("expected 2 configured content ids, got %d (%v)", len(cfg), cfg)
	}
	if got, ok := cfg["kb-doc-wiki"]; !ok || got != true {
		t.Fatalf("kb-doc-wiki=%v ok=%v", got, ok)
	}
	if got, ok := cfg["kb-metrics"]; !ok || got != false {
		t.Fatalf("kb-metrics=%v ok=%v", got, ok)
	}
}

// Verifies that a [knowledge-content] value in a locally-merged config
// (simulating config.local.toml being merged on top of config.toml, the
// same viper.ReadInConfig + MergeInConfig flow used by LoadConfig) overrides
// the base value, matching how config.local.toml takes precedence.
func TestGetKnowledgeContentConfigLocalOverrideWins(t *testing.T) {
	oldConfig := AppConfig
	oldViper := appConfigViper
	t.Cleanup(func() {
		AppConfig = oldConfig
		appConfigViper = oldViper
		viper.Reset()
	})
	viper.Reset()
	appConfigViper = viper.New()
	viper.SetConfigType("toml")

	const base = `
[knowledge-content]
kb-metrics = true
kb-doc-wiki = true
`
	const local = `
[knowledge-content]
kb-metrics = false
`
	if err := viper.ReadConfig(strings.NewReader(base)); err != nil {
		t.Fatalf("read base config: %v", err)
	}
	if err := viper.MergeConfig(strings.NewReader(local)); err != nil {
		t.Fatalf("merge local config: %v", err)
	}
	if err := viper.Unmarshal(&AppConfig); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	appConfigViper = viper.GetViper()

	cfg := GetKnowledgeContentConfig()
	if got, ok := cfg["kb-metrics"]; !ok || got != false {
		t.Fatalf("expected local override to disable kb-metrics, got %v ok=%v", got, ok)
	}
	if got, ok := cfg["kb-doc-wiki"]; !ok || got != true {
		t.Fatalf("expected kb-doc-wiki to remain enabled from base config, got %v ok=%v", got, ok)
	}
}
