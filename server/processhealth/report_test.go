package processhealth

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestReporterAtomicConcurrentPublication(t *testing.T) {
	dir := t.TempDir()
	r, err := NewReporterAt(dir, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Set(Blocked, "missing_asset", "path/to/asset", "Required asset does not exist", OperatorAction); err != nil {
		t.Fatal(err)
	}
	since := r.Snapshot().Since
	var wg sync.WaitGroup
	for n := 0; n < 10; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for k := 0; k < 20; k++ {
				if err := r.Refresh(); err != nil {
					t.Error(err)
				}
				if _, err := ReadReport(dir, "worker", os.Getpid()); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	got, err := ReadReport(dir, "worker", os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	if got.State != Blocked || !got.Since.Equal(since) {
		t.Fatal("refresh erased fatal cause or incident start")
	}
}

func TestHealthJSONReadIsBoundedAndRejectsTrailingDocuments(t *testing.T) {
	dir := t.TempDir()
	r, err := NewReporterAt(dir, "worker")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, fmt.Sprintf("worker-%d.json", os.Getpid()))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, []byte(" {}")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(dir, "worker", os.Getpid()); err == nil {
		t.Fatal("trailing document accepted")
	}
	if err := os.WriteFile(path, []byte(strings.Repeat(" ", 64*1024+1)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(dir, "worker", os.Getpid()); err == nil {
		t.Fatal("oversized report accepted")
	}
	_ = r
}

func TestReporterDisabledWithoutDirectory(t *testing.T) {
	t.Setenv("PROCESS_HEALTH_DIR", "")
	r, err := NewReporter("worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Set(Healthy, "", "", "", None); err != nil {
		t.Fatal(err)
	}
}

func TestReporterRejectsUnsafeNamesAndInvalidState(t *testing.T) {
	dir := t.TempDir()
	if _, err := NewReporterAt(dir, "../other"); err == nil {
		t.Fatal("path traversal accepted")
	}
	r, err := NewReporterAt(dir, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Set(Blocked, "", "", "", OperatorAction); err == nil {
		t.Fatal("unclassified fatal accepted")
	}
	if r.Snapshot().State != Starting {
		t.Fatal("invalid state modified reporter")
	}
	data := []byte(`{"service":"different"}`)
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("worker-%d.json", os.Getpid())), data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(dir, "worker", os.Getpid()); err == nil {
		t.Fatal("invalid report accepted")
	}
}

func TestSeparateProcessesDoNotOverwriteReports(t *testing.T) {
	if dir := os.Getenv("HEALTH_TEST_CHILD"); dir != "" {
		if _, err := NewReporterAt(dir, "worker"); err != nil {
			t.Fatal(err)
		}
		return
	}
	dir := t.TempDir()
	var pids []int
	for i := 0; i < 2; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestSeparateProcessesDoNotOverwriteReports$")
		cmd.Env = append(os.Environ(), "HEALTH_TEST_CHILD="+dir)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		pids = append(pids, cmd.Process.Pid)
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	for _, pid := range pids {
		path := filepath.Join(dir, fmt.Sprintf("worker-%d.json", pid))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing report for process %d: %v", pid, err)
		}
	}
}

func TestReadReportUsesMatchingPIDAndSupportsLegacy(t *testing.T) {
	dir := t.TempDir()
	r, err := NewReporterAt(dir, "worker")
	if err != nil {
		t.Fatal(err)
	}
	pid := r.Snapshot().PID
	path := filepath.Join(dir, fmt.Sprintf("worker-%d.json", pid))
	legacy := filepath.Join(dir, "worker.json")
	if err := os.Rename(path, legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(dir, "worker", pid); err != nil {
		t.Fatalf("legacy: %v", err)
	}
	if _, err := ReadReport(dir, "worker", pid+1); err == nil {
		t.Fatal("accepted another process's legacy report")
	}
	if err := os.Rename(legacy, filepath.Join(dir, fmt.Sprintf("worker-%d.json", pid+1))); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadReport(dir, "worker", pid+1); err == nil {
		t.Fatal("accepted report with mismatched PID")
	}
}
