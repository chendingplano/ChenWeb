package codemonitor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestCollectPackageSkipsTrackedFileDeletedFromWorkingTree(t *testing.T) {
	root := t.TempDir()
	gitRun := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	gitRun("init", "-q")
	if err := os.Mkdir(filepath.Join(root, "server"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "server", "keep.go"), []byte("package server\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "server", "deleted.go"), []byte("package server\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitRun("add", "server")
	if err := os.Remove(filepath.Join(root, "server", "deleted.go")); err != nil {
		t.Fatal(err)
	}

	snapshot, err := collectPackage(root, "server")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot != (Snapshot{TotalLines: 1, BackendFiles: 1}) {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestCountPhysicalLines(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          int
	}{
		{"empty", "", 0}, {"newline terminated", "a\nb\n", 2}, {"no final newline", "a\nb", 2}, {"blank lines", "\n\n", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.go")
			if err := os.WriteFile(path, []byte(tc.content), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := countLines(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("countLines() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestClassifyExtension(t *testing.T) {
	for ext, want := range map[string]fileKind{".svelte": frontendFile, ".ts": frontendFile, ".go": backendFile, ".sql": backendFile, ".md": ignoredFile} {
		if got := classifyExtension(ext); got != want {
			t.Errorf("classifyExtension(%q) = %v, want %v", ext, got, want)
		}
	}
}

func TestDateRange(t *testing.T) {
	loc, _ := time.LoadLocation("America/Chicago")
	today := time.Date(2026, 3, 31, 12, 0, 0, 0, loc)
	start, end, err := dateRange("last_6_months", today, loc)
	if err != nil {
		t.Fatal(err)
	}
	if start.Format("2006-01-02") != "2025-09-30" || end.Format("2006-01-02") != "2026-03-31" {
		t.Fatalf("range = %s..%s", start, end)
	}
	start, end, err = dateRange("last_month", today, loc)
	if err != nil {
		t.Fatal(err)
	}
	if start.Format("2006-01-02") != "2026-03-02" || end.Format("2006-01-02") != "2026-03-31" {
		t.Fatalf("30-day range = %s..%s", start, end)
	}
}

func TestNextRunDST(t *testing.T) {
	loc, _ := time.LoadLocation("America/Chicago")
	now := time.Date(2026, 3, 7, 23, 0, 0, 0, loc)
	next := nextRun(now, loc)
	if next.Hour() != 3 || next.Minute() != 0 || next.Day() != 8 {
		t.Fatalf("next run = %s", next)
	}
	if next.Sub(now) != 3*time.Hour {
		t.Fatalf("next run duration = %s", next.Sub(now))
	}
}
