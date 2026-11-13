package systemmonitor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTailBoundedAndMissing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "app.log")
	if err := os.WriteFile(p, []byte(strings.Repeat("x", 100)+"\nlast line\n"), 0600); err != nil {
		t.Fatal(err)
	}
	got, truncated, err := readTail(p, 20)
	if err != nil || !truncated || got != "last line\n" {
		t.Fatalf("got %q, %v, %v", got, truncated, err)
	}
	_, _, err = readTail(filepath.Join(dir, "missing"), 20)
	if !os.IsNotExist(err) {
		t.Fatalf("missing file: %v", err)
	}
}

func TestSourcePathUsesConfiguredRotatingFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DOC_PROCESSOR_LOG_DIR", dir)
	p := filepath.Join(dir, "log_00")
	if err := os.WriteFile(p, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := sourcePath(source{id: "doc-processor", env: "DOC_PROCESSOR_LOG_DIR", fallback: "doc-processor", directory: true}); got != p {
		t.Fatalf("got %q, want %q", got, p)
	}
}
