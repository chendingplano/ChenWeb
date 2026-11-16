package systemmonitor

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRestartService(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A pid that is no longer alive; the stop must be followed by a start.
	if err := os.WriteFile(filepath.Join(root, ".cache", "doc-service.pid"), []byte("4242\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldRun, oldAlive, oldWait := runMise, pidAlive, stopWait
	t.Cleanup(func() { runMise, pidAlive, stopWait = oldRun, oldAlive, oldWait })
	stopWait = time.Second
	pidAlive = func(pid int) bool { return false }
	var calls []string
	runMise = func(_ context.Context, dir string, task string) ([]byte, error) {
		calls = append(calls, filepath.Base(dir)+":"+task)
		return []byte(task + " ok\n"), nil
	}

	out, err := restartService(context.Background(), root, restartTargets["doc-service"])
	if err != nil {
		t.Fatalf("restartService: %v", err)
	}
	if len(calls) != 2 || calls[0] != "doc-service:doc-service-stop" || calls[1] != "doc-service:doc-service-start" {
		t.Fatalf("calls = %v", calls)
	}
	if string(out) != "doc-service-stop ok\ndoc-service-start ok\n" {
		t.Fatalf("output = %q", out)
	}
}

func TestRestartServiceWaitsForOldProcess(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldRun, oldAlive, oldWait := runMise, pidAlive, stopWait
	t.Cleanup(func() { runMise, pidAlive, stopWait = oldRun, oldAlive, oldWait })
	stopWait = 50 * time.Millisecond
	pidAlive = func(pid int) bool { return true } // never exits
	var calls []string
	runMise = func(_ context.Context, _ string, task string) ([]byte, error) {
		calls = append(calls, task)
		return nil, nil
	}
	if err := os.WriteFile(filepath.Join(root, ".cache", "doc-processor.pid"), []byte(strconv.Itoa(99)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := restartService(context.Background(), root, restartTargets["doc-processor"]); err == nil {
		t.Fatal("restart succeeded while the old process was still alive")
	}
	if len(calls) != 1 || calls[0] != "doc-processor-stop" {
		t.Fatalf("start ran after a failed stop wait: %v", calls)
	}
}

func TestRestartTargetsAreNotTheWholeList(t *testing.T) {
	for _, id := range []string{"chenweb", "kratos", "nats", "caddy", "pdf-parser", "deepseek-watcher", "sshd", "doc-service;rm"} {
		if _, ok := restartTargets[id]; ok {
			t.Errorf("%q should not be restartable from the monitor", id)
		}
	}
	for id, target := range restartTargets {
		if _, ok := restartLocks[id]; !ok {
			t.Errorf("no restart lock for %q", id)
		}
		if target.name != id {
			t.Errorf("%q: task prefix %q must match the source id", id, target.name)
		}
	}
}
