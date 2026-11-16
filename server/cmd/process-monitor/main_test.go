package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllowlistRejectsCommandInjectionAndTemplates(t *testing.T) {
	for _, unit := range []string{"--help", "../../nginx", "worker@instance.service", "nginx.service;touch /tmp/unsafe", "*.service"} {
		path := filepath.Join(t.TempDir(), "services.json")
		if err := os.WriteFile(path, []byte(`[{"name":"worker","unit":"`+unit+`"}]`), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := readServices(path); err == nil {
			t.Fatalf("unsafe unit accepted %q", unit)
		}
	}
}

func TestMonitorLockExcludesConcurrentRuns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitor.lock")
	unlock, err := lock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if second, err := lock(path); err == nil {
		second()
		t.Fatal("concurrent monitor entered critical section")
	}
}
