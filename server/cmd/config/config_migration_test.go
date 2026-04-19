package config

import (
	"os"
	"path/filepath"
	"testing"
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
