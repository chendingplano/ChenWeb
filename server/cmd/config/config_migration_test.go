package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestResolveMigrationDir_UsesNearestExistingAncestor(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "server", "cmd", "doc-processor")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	want := filepath.Join(root, "project_migrations")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatalf("mkdir migration dir: %v", err)
	}

	got := resolveMigrationDir(configDir, "project_migrations")
	if got != want {
		t.Fatalf("resolveMigrationDir() = %q, want %q", got, want)
	}
}

func TestFrontendProjectsToMonitorConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[frontend]\nprojects-to-monitor = [\"web/src\", \"server\"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v := viper.New()
	v.SetConfigFile(path)
	if err := v.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	var got FrontendConfigSection
	if err := v.UnmarshalKey("frontend", &got); err != nil {
		t.Fatal(err)
	}
	if len(got.ProjectsToMonitor) != 2 || got.ProjectsToMonitor[0] != "web/src" || got.ProjectsToMonitor[1] != "server" {
		t.Fatalf("projects-to-monitor = %#v", got.ProjectsToMonitor)
	}
}

func TestResolveMigrationDir_PrefersConfigLocalWhenExists(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "cmd", "x")
	local := filepath.Join(configDir, "project_migrations")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatalf("mkdir local migration dir: %v", err)
	}

	got := resolveMigrationDir(configDir, "project_migrations")
	if got != local {
		t.Fatalf("resolveMigrationDir() = %q, want %q", got, local)
	}
}
