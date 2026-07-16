package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// Verifies that GetKnowledgeMenusConfig returns an empty map when no
// [knowledge-menus] section is present, so the Wiki sidebar menu defaults to
// fully enabled.
func TestGetKnowledgeMenusConfigDefaultsToEmpty(t *testing.T) {
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

	cfg := GetKnowledgeMenusConfig()
	if len(cfg) != 0 {
		t.Fatalf("expected no configured menu ids, got %v", cfg)
	}
}

// Verifies that a [knowledge-menus] TOML section unmarshals id->bool entries
// into the KnowledgeMenus map and is returned by GetKnowledgeMenusConfig.
func TestGetKnowledgeMenusConfigUnmarshalsIdBooleans(t *testing.T) {
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
[knowledge-menus]
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

	cfg := GetKnowledgeMenusConfig()
	if len(cfg) != 2 {
		t.Fatalf("expected 2 configured menu ids, got %d (%v)", len(cfg), cfg)
	}
	if got, ok := cfg["kb-doc-wiki"]; !ok || got != true {
		t.Fatalf("kb-doc-wiki=%v ok=%v", got, ok)
	}
	if got, ok := cfg["kb-metrics"]; !ok || got != false {
		t.Fatalf("kb-metrics=%v ok=%v", got, ok)
	}
}

// Verifies that a [knowledge-menus] value in a locally-merged config
// (simulating config.local.toml being merged on top of config.toml, the
// same viper.ReadInConfig + MergeInConfig flow used by LoadConfig) overrides
// the base value, matching how config.local.toml takes precedence.
func TestGetKnowledgeMenusConfigLocalOverrideWins(t *testing.T) {
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
[knowledge-menus]
kb-metrics = true
kb-doc-wiki = true
`
	const local = `
[knowledge-menus]
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

	cfg := GetKnowledgeMenusConfig()
	if got, ok := cfg["kb-metrics"]; !ok || got != false {
		t.Fatalf("expected local override to disable kb-metrics, got %v ok=%v", got, ok)
	}
	if got, ok := cfg["kb-doc-wiki"]; !ok || got != true {
		t.Fatalf("expected kb-doc-wiki to remain enabled from base config, got %v ok=%v", got, ok)
	}
}
