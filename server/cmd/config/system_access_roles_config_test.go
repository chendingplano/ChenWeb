package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestGetAccessRolesFallsBackWhenSectionMissing(t *testing.T) {
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

	fallback := []string{"admin", "root", "guest"}
	got := GetAccessRoles(fallback)
	if !reflect.DeepEqual(got, fallback) {
		t.Fatalf("expected fallback %v, got %v", fallback, got)
	}
}

func TestGetAccessRolesUnmarshalsAndNormalizesConfiguredRoles(t *testing.T) {
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
[system]
access_roles = [" Admin ", "DEV", "k_engineer", "dev", "", "trial"]
`
	if err := viper.ReadConfig(strings.NewReader(sample)); err != nil {
		t.Fatalf("read config: %v", err)
	}
	if err := viper.Unmarshal(&AppConfig); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	appConfigViper = viper.GetViper()

	got := GetAccessRoles([]string{"fallback"})
	want := []string{"admin", "dev", "k_engineer", "trial"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected normalized roles %v, got %v", want, got)
	}
}

func TestGetAccessRolesFallsBackWhenConfiguredListEmpty(t *testing.T) {
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
[system]
access_roles = ["admin", "dev"]
`
	const local = `
[system]
access_roles = []
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

	fallback := []string{"admin", "root", "guest"}
	got := GetAccessRoles(fallback)
	if !reflect.DeepEqual(got, fallback) {
		t.Fatalf("expected empty configured list to fall back to %v, got %v", fallback, got)
	}
}
