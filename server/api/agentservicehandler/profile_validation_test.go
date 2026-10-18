package agentservicehandler

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func validProfileCandidate() ProfileCandidate {
	return ProfileCandidate{
		Slug: "knowledge-guide", FriendlyName: "Guide", Description: "Answers questions",
		Provider: "anthropic", Model: "claude", ThinkingLevel: "medium",
		PromptFile: "prompt-agent-knowledge-guide-v1.md", AllowedKnowledgeStores: []string{"Research"},
		AllowedDocumentGroups: []string{"published"}, PermissionDefault: PermissionAsk,
		Limits: ProfileLimits{MaxToolCalls: 12, MaxElapsed: 90 * time.Second, MaxOutputTokens: 1200,
			MaxEvidenceBytes: 65536, MaxHistoryTokens: 32000, MaxHistoryToolResultBytes: 4096},
		Enabled: true,
	}
}

func TestValidateProfileCandidateRejectsInvalidInputs(t *testing.T) {
	promptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptDir, "prompt-agent-knowledge-guide-v1.md"), []byte("Use evidence."), 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(*ProfileCandidate)
	}{
		{"unsupported slug", func(c *ProfileCandidate) { c.Slug = "other" }},
		{"missing prompt", func(c *ProfileCandidate) { c.PromptFile = "prompt-agent-missing-v1.md" }},
		{"empty prompt", func(c *ProfileCandidate) { c.PromptFile = "prompt-agent-knowledge-guide-v1.md" }},
		{"prompt traversal", func(c *ProfileCandidate) { c.PromptFile = "../prompt-agent-guide-v1.md" }},
		{"prompt outside allowlist", func(c *ProfileCandidate) { c.PromptFile = "prompt-agent-other-v1.md" }},
		{"prompt for different service", func(c *ProfileCandidate) { c.PromptFile = "prompt-agent-problem-diagnosis-v1.md" }},
		{"blank provider", func(c *ProfileCandidate) { c.Provider = "  " }},
		{"blank model", func(c *ProfileCandidate) { c.Model = "" }},
		{"invalid permission", func(c *ProfileCandidate) { c.PermissionDefault = "never" }},
		{"invalid thinking", func(c *ProfileCandidate) { c.ThinkingLevel = "ultra" }},
		{"empty stores", func(c *ProfileCandidate) { c.AllowedKnowledgeStores = []string{" ", ""} }},
		{"empty document group", func(c *ProfileCandidate) { c.AllowedDocumentGroups = []string{"published", " "} }},
		{"zero tool calls", func(c *ProfileCandidate) { c.Limits.MaxToolCalls = 0 }},
		{"over max tool calls", func(c *ProfileCandidate) { c.Limits.MaxToolCalls = 65 }},
		{"zero elapsed", func(c *ProfileCandidate) { c.Limits.MaxElapsed = 0 }},
		{"over max elapsed", func(c *ProfileCandidate) { c.Limits.MaxElapsed = 601 * time.Second }},
		{"zero output tokens", func(c *ProfileCandidate) { c.Limits.MaxOutputTokens = 0 }},
		{"over max output tokens", func(c *ProfileCandidate) { c.Limits.MaxOutputTokens = 8193 }},
		{"zero evidence bytes", func(c *ProfileCandidate) { c.Limits.MaxEvidenceBytes = 0 }},
		{"over max evidence bytes", func(c *ProfileCandidate) { c.Limits.MaxEvidenceBytes = 1_048_577 }},
		{"zero history tokens", func(c *ProfileCandidate) { c.Limits.MaxHistoryTokens = 0 }},
		{"over max history tokens", func(c *ProfileCandidate) { c.Limits.MaxHistoryTokens = 128_001 }},
		{"zero carried tool result bytes", func(c *ProfileCandidate) { c.Limits.MaxHistoryToolResultBytes = 0 }},
		{"over max carried tool result bytes", func(c *ProfileCandidate) { c.Limits.MaxHistoryToolResultBytes = 32_769 }},
	}
	if err := os.WriteFile(filepath.Join(promptDir, "prompt-agent-knowledge-guide-v1.md"), []byte(" \n "), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := validProfileCandidate()
			tt.change(&candidate)
			if _, err := ValidateProfileCandidate(promptDir, candidate); err == nil {
				t.Fatal("ValidateProfileCandidate() error = nil, want error")
			}
		})
	}
}

func TestValidateProfileCandidateNormalizesAndSnapshots(t *testing.T) {
	promptDir := t.TempDir()
	prompt := []byte("Use evidence carefully.\n")
	if err := os.WriteFile(filepath.Join(promptDir, "prompt-agent-knowledge-guide-v1.md"), prompt, 0600); err != nil {
		t.Fatal(err)
	}
	candidate := validProfileCandidate()
	candidate.Provider = "  openai  "
	candidate.Model = " gpt-test "
	candidate.AllowedKnowledgeStores = []string{" Research ", "", "Analytics"}
	candidate.AllowedDocumentGroups = []string{" published ", "reviewed"}
	candidate.PilotUsers = []string{" alice ", "alice", "Alice", ""}
	got, err := ValidateProfileCandidate(promptDir, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile.Provider != "openai" || got.Profile.Model != "gpt-test" || got.Profile.ProviderDisclosure != "Messages and retrieved evidence are sent to the openai model provider." {
		t.Fatalf("provider fields = %q, %q, %q", got.Profile.Provider, got.Profile.Model, got.Profile.ProviderDisclosure)
	}
	hash := sha256.Sum256(prompt)
	if got.Profile.SystemPrompt != string(prompt) || got.PromptRef != candidate.PromptFile || got.PromptHash != hex.EncodeToString(hash[:]) {
		t.Fatalf("prompt snapshot metadata = %#v", got)
	}
	if !reflect.DeepEqual(got.Profile.PilotUsers, []string{"alice", "Alice"}) {
		t.Fatalf("pilot users = %#v", got.Profile.PilotUsers)
	}
	if !reflect.DeepEqual(got.Profile.AllowedKnowledgeStores, []string{"Research", "Analytics"}) {
		t.Fatalf("stores = %#v", got.Profile.AllowedKnowledgeStores)
	}
	if !reflect.DeepEqual(got.Profile.AllowedTools, knowledgeToolNames) {
		t.Fatalf("tools = %#v", got.Profile.AllowedTools)
	}
}

func TestValidateProfileCandidateAcceptsLimitBoundaries(t *testing.T) {
	promptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptDir, "prompt-agent-knowledge-guide-v1.md"), []byte("prompt"), 0600); err != nil {
		t.Fatal(err)
	}
	candidate := validProfileCandidate()
	candidate.Limits = ProfileLimits{MaxToolCalls: 64, MaxElapsed: 600 * time.Second, MaxOutputTokens: 8192,
		MaxEvidenceBytes: 1_048_576, MaxHistoryTokens: 128_000, MaxHistoryToolResultBytes: 32_768}
	if _, err := ValidateProfileCandidate(promptDir, candidate); err != nil {
		t.Fatalf("maximum limits rejected: %v", err)
	}
	candidate.Limits = ProfileLimits{MaxToolCalls: 1, MaxElapsed: time.Second, MaxOutputTokens: 1,
		MaxEvidenceBytes: 1, MaxHistoryTokens: 1, MaxHistoryToolResultBytes: 1}
	if _, err := ValidateProfileCandidate(promptDir, candidate); err != nil {
		t.Fatalf("minimum limits rejected: %v", err)
	}
}

func TestValidateProfileCandidatePreservesDisabledState(t *testing.T) {
	promptDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(promptDir, "prompt-agent-knowledge-guide-v1.md"), []byte("prompt"), 0600); err != nil {
		t.Fatal(err)
	}
	candidate := validProfileCandidate()
	candidate.Enabled = false
	got, err := ValidateProfileCandidate(promptDir, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if got.Profile.Enabled {
		t.Fatal("disabled state was not preserved")
	}
}

func TestValidateProfileCandidateRejectsPromptSymlink(t *testing.T) {
	promptDir := t.TempDir()
	target := filepath.Join(promptDir, "target.md")
	if err := os.WriteFile(target, []byte("prompt"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(promptDir, "prompt-agent-knowledge-guide-v1.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ValidateProfileCandidate(promptDir, validProfileCandidate()); err == nil {
		t.Fatal("ValidateProfileCandidate() accepted a prompt symlink")
	}
}
