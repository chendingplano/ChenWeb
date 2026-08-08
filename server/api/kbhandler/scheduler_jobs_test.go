package kbhandler

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultSchedulerRegistryIncludesTerminologyRefresh(t *testing.T) {
	reg := DefaultSchedulerRegistry()
	desc, ok := reg["terminology_refresh"]
	if !ok {
		t.Fatal("registry missing terminology_refresh job")
	}
	if desc.Label != "Refresh External Terminology Resources" {
		t.Fatalf("label = %q, want Refresh External Terminology Resources", desc.Label)
	}
	if desc.Run == nil {
		t.Fatal("terminology_refresh must have a Run func")
	}
}

func TestRunTerminologyRefreshJobRequiresDirectory(t *testing.T) {
	t.Setenv("TERMINOLOGY_DIR", "")
	t.Setenv("DATA_HOME_DIR", "")
	_, err := runTerminologyRefreshJob(context.Background(), map[string]any{"sources": "wikidata"}, nil)
	if err == nil || !strings.Contains(err.Error(), "TERMINOLOGY_DIR") {
		t.Fatalf("err = %v, want missing directory error", err)
	}
}

func TestRunTerminologyRefreshJobReportsPerSourceErrors(t *testing.T) {
	t.Setenv("TERMINOLOGY_DIR", t.TempDir())
	got, err := runTerminologyRefreshJob(context.Background(), map[string]any{"sources": "no-such,iec-60050-845"}, nil)
	if err != nil {
		t.Fatalf("runTerminologyRefreshJob: %v", err)
	}
	sources, ok := got["sources"].([]map[string]any)
	if !ok || len(sources) != 2 {
		t.Fatalf("sources = %#v, want 2 per-source entries", got["sources"])
	}
	if sources[0]["source"] != "no-such" || !strings.Contains(sources[0]["error"].(string), "unknown terminology resource") {
		t.Fatalf("entry 0 = %#v", sources[0])
	}
	if sources[1]["source"] != "iec-60050-845" || !strings.Contains(sources[1]["error"].(string), "requires permission") {
		t.Fatalf("entry 1 = %#v", sources[1])
	}
}
