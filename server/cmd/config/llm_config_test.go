package config

import "testing"

func TestGetLLMConfigDefaults(t *testing.T) {
	old := AppConfig
	t.Cleanup(func() { AppConfig = old })

	AppConfig = AppConfigDef{}

	got := GetLLMConfig()
	if got.WorkspaceTimezone != "America/Chicago" {
		t.Fatalf("WorkspaceTimezone = %q, want America/Chicago", got.WorkspaceTimezone)
	}
	if got.UsageRetentionDays != 30 {
		t.Fatalf("UsageRetentionDays = %d, want 30", got.UsageRetentionDays)
	}
	if got.ArchiveRoot != "Data/llm-logs" {
		t.Fatalf("ArchiveRoot = %q, want Data/llm-logs", got.ArchiveRoot)
	}
	if got.ReconciliationRunHour != 2 {
		t.Fatalf("ReconciliationRunHour = %d, want 2", got.ReconciliationRunHour)
	}
}

func TestGetLLMConfigPreservesConfiguredValues(t *testing.T) {
	old := AppConfig
	t.Cleanup(func() { AppConfig = old })

	AppConfig.LLM = LLMConfig{
		WorkspaceTimezone:     "UTC",
		UsageRetentionDays:    90,
		ArchiveRoot:           "/tmp/llm-logs",
		ReconciliationRunHour: 5,
	}

	got := GetLLMConfig()
	if got.WorkspaceTimezone != "UTC" {
		t.Fatalf("WorkspaceTimezone = %q, want UTC", got.WorkspaceTimezone)
	}
	if got.UsageRetentionDays != 90 {
		t.Fatalf("UsageRetentionDays = %d, want 90", got.UsageRetentionDays)
	}
	if got.ArchiveRoot != "/tmp/llm-logs" {
		t.Fatalf("ArchiveRoot = %q, want /tmp/llm-logs", got.ArchiveRoot)
	}
	if got.ReconciliationRunHour != 5 {
		t.Fatalf("ReconciliationRunHour = %d, want 5", got.ReconciliationRunHour)
	}
}
