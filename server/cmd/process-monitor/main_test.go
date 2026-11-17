package main

import (
	"context"
	"fmt"

	"github.com/chendingplano/deepdoc/server/processhealth"
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

func TestObserveReadsReportForSystemdProcess(t *testing.T) {
	reports := t.TempDir()
	reporter, err := processhealth.NewReporterAt(reports, "worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := reporter.Set(processhealth.Blocked, "configuration_invalid", "config", "bad configuration", processhealth.OperatorAction); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	systemctl := filepath.Join(bin, "systemctl")
	script := fmt.Sprintf("#!/bin/sh\nprintf 'ActiveState=failed\\nMainPID=0\\nExecMainPID=%d\\n'\n", os.Getpid())
	if err := os.WriteFile(systemctl, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	o := observe(context.Background(), reports, processhealth.Service{Name: "worker", Unit: "worker.service", RequiresReport: true})
	if o.ReportError != nil || o.Report.PID != os.Getpid() || o.Report.Reason != "configuration_invalid" {
		t.Fatalf("report: %+v, error: %v", o.Report, o.ReportError)
	}
}
