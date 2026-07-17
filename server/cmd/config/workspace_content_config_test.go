package config

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
)

// Verifies that GetWorkspaceContentConfig returns an empty map when no
// [workspace-content] section is present, so the workspace page defaults to
// fully enabled.
func TestGetWorkspaceContentConfigDefaultsToEmpty(t *testing.T) {
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

	cfg := GetWorkspaceContentConfig()
	if len(cfg) != 0 {
		t.Fatalf("expected no configured workspace content ids, got %v", cfg)
	}
}

// Verifies that a [workspace-content] TOML section unmarshals id->bool
// entries into the WorkspaceContent map and is returned by
// GetWorkspaceContentConfig.
func TestGetWorkspaceContentConfigUnmarshalsIdBooleans(t *testing.T) {
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
[workspace-content]
workflows = false
ws-announcements = true
`
	if err := viper.ReadConfig(strings.NewReader(sample)); err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := viper.Unmarshal(&AppConfig); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	appConfigViper = viper.GetViper()

	cfg := GetWorkspaceContentConfig()
	if len(cfg) != 2 {
		t.Fatalf("expected 2 configured workspace content ids, got %d (%v)", len(cfg), cfg)
	}
	if got, ok := cfg["workflows"]; !ok || got != false {
		t.Fatalf("workflows=%v ok=%v", got, ok)
	}
	if got, ok := cfg["ws-announcements"]; !ok || got != true {
		t.Fatalf("ws-announcements=%v ok=%v", got, ok)
	}
}

// Verifies that a [workspace-content] value in a locally-merged config
// (simulating config.local.toml being merged on top of config.toml, the
// same viper.ReadInConfig + MergeInConfig flow used by LoadConfig) overrides
// the base value, matching how config.local.toml takes precedence.
func TestGetWorkspaceContentConfigLocalOverrideWins(t *testing.T) {
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
[workspace-content]
workflows = true
agents = true
`
	const local = `
[workspace-content]
workflows = false
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

	cfg := GetWorkspaceContentConfig()
	if got, ok := cfg["workflows"]; !ok || got != false {
		t.Fatalf("expected local override to disable workflows, got %v ok=%v", got, ok)
	}
	if got, ok := cfg["agents"]; !ok || got != true {
		t.Fatalf("expected agents to remain enabled from base config, got %v ok=%v", got, ok)
	}
}
