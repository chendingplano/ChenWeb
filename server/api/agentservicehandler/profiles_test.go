package agentservicehandler

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func repositoryPromptDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../../../prompts"))
}

func TestLoadProfileRegistryLoadsTwoIndependentVersionedProfiles(t *testing.T) {
	registry, err := LoadProfileRegistry(repositoryPromptDir(t))
	if err != nil {
		t.Fatalf("LoadProfileRegistry() error = %v", err)
	}

	knowledge, err := registry.Resolve("knowledge-guide", "user-1")
	if err != nil {
		t.Fatalf("Resolve(knowledge-guide) error = %v", err)
	}
	diagnostics, err := registry.Resolve("problem-diagnostics", "user-1")
	if err != nil {
		t.Fatalf("Resolve(problem-diagnostics) error = %v", err)
	}

	if knowledge.Slug != "knowledge-guide" || diagnostics.Slug != "problem-diagnostics" {
		t.Fatalf("unexpected slugs: %q, %q", knowledge.Slug, diagnostics.Slug)
	}
	if knowledge.Version != "v1" || diagnostics.Version != "v1" {
		t.Fatalf("unexpected versions: %q, %q", knowledge.Version, diagnostics.Version)
	}
	if knowledge.FriendlyName == diagnostics.FriendlyName || knowledge.SystemPrompt == diagnostics.SystemPrompt {
		t.Fatal("profiles must have independent names and prompts")
	}
	for _, profile := range []PiProfile{knowledge, diagnostics} {
		if profile.Provider == "" || profile.Model == "" || profile.ProviderDisclosure == "" {
			t.Fatalf("provider metadata incomplete: %+v", profile)
		}
		if profile.PermissionDefault != PermissionAuto {
			t.Fatalf("permission default = %q, want auto", profile.PermissionDefault)
		}
		if len(profile.AllowedTools) != 5 || len(profile.AllowedKnowledgeStores) == 0 {
			t.Fatalf("profile scopes incomplete: %+v", profile)
		}
		if profile.Limits.MaxToolCalls <= 0 || profile.Limits.MaxElapsed <= 0 || profile.Limits.MaxOutputTokens <= 0 {
			t.Fatalf("profile limits incomplete: %+v", profile.Limits)
		}
		for _, phrase := range []string{"untrusted evidence", "cite", "general model knowledge"} {
			if !strings.Contains(strings.ToLower(profile.SystemPrompt), phrase) {
				t.Fatalf("prompt %q missing %q", profile.PromptFile, phrase)
			}
		}
	}
	if !strings.Contains(strings.ToLower(diagnostics.SystemPrompt), "medical") ||
		!strings.Contains(strings.ToLower(diagnostics.SystemPrompt), "read-only") {
		t.Fatal("diagnostics prompt must state high-stakes and read-only limits")
	}
}

func TestLoadProfileRegistryAppliesEnvironmentOverridesPerProfile(t *testing.T) {
	t.Setenv("PI_KNOWLEDGE_GUIDE_PROVIDER", "openai")
	t.Setenv("PI_KNOWLEDGE_GUIDE_MODEL", "gpt-test")
	t.Setenv("PI_KNOWLEDGE_GUIDE_PROVIDER_DISCLOSURE", "Evidence is sent to Test Provider.")
	t.Setenv("PI_KNOWLEDGE_GUIDE_THINKING_LEVEL", "high")
	t.Setenv("PI_KNOWLEDGE_GUIDE_PERMISSION_DEFAULT", "ask")
	t.Setenv("PI_KNOWLEDGE_GUIDE_ALLOWED_STORES", "Research, Manuals")
	t.Setenv("PI_KNOWLEDGE_GUIDE_ALLOWED_DOCUMENT_GROUPS", "published, reviewed")
	t.Setenv("PI_KNOWLEDGE_GUIDE_MAX_TOOL_CALLS", "7")
	t.Setenv("PI_KNOWLEDGE_GUIDE_MAX_ELAPSED_SECONDS", "45")
	t.Setenv("PI_KNOWLEDGE_GUIDE_MAX_OUTPUT_TOKENS", "900")
	t.Setenv("PI_KNOWLEDGE_GUIDE_PILOT_USERS", "user-1,user-2")

	registry, err := LoadProfileRegistry(repositoryPromptDir(t))
	if err != nil {
		t.Fatalf("LoadProfileRegistry() error = %v", err)
	}
	knowledge, err := registry.Resolve("knowledge-guide", "user-1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	diagnostics, err := registry.Resolve("problem-diagnostics", "any-user")
	if err != nil {
		t.Fatalf("Resolve(problem-diagnostics) error = %v", err)
	}

	if knowledge.Provider != "openai" || knowledge.Model != "gpt-test" || knowledge.ThinkingLevel != "high" {
		t.Fatalf("model overrides not applied: %+v", knowledge)
	}
	if knowledge.ProviderDisclosure != "Evidence is sent to Test Provider." || knowledge.PermissionDefault != PermissionAsk {
		t.Fatalf("disclosure/permission overrides not applied: %+v", knowledge)
	}
	if strings.Join(knowledge.AllowedKnowledgeStores, ",") != "Research,Manuals" ||
		strings.Join(knowledge.AllowedDocumentGroups, ",") != "published,reviewed" {
		t.Fatalf("scope overrides not applied: %+v", knowledge)
	}
	if knowledge.Limits.MaxToolCalls != 7 || knowledge.Limits.MaxElapsed != 45*time.Second || knowledge.Limits.MaxOutputTokens != 900 {
		t.Fatalf("limit overrides not applied: %+v", knowledge.Limits)
	}
	if diagnostics.Provider == "openai" || diagnostics.Model == "gpt-test" {
		t.Fatal("knowledge-guide overrides leaked into problem-diagnostics")
	}
	if _, err := registry.Resolve("knowledge-guide", "not-a-pilot"); !errors.Is(err, ErrProfileNotAvailable) {
		t.Fatalf("pilot restriction error = %v", err)
	}
}

func TestProfileRegistryRejectsUnknownAndDisabledProfiles(t *testing.T) {
	t.Setenv("PI_PROBLEM_DIAGNOSTICS_ENABLED", "false")
	registry, err := LoadProfileRegistry(repositoryPromptDir(t))
	if err != nil {
		t.Fatalf("LoadProfileRegistry() error = %v", err)
	}
	if _, err := registry.Resolve("unknown", "user-1"); !errors.Is(err, ErrProfileNotFound) {
		t.Fatalf("unknown profile error = %v", err)
	}
	if _, err := registry.Resolve("problem-diagnostics", "user-1"); !errors.Is(err, ErrProfileDisabled) {
		t.Fatalf("disabled profile error = %v", err)
	}
	if _, err := registry.ResolveVersion("problem-diagnostics", "v1", "user-1"); !errors.Is(err, ErrProfileDisabled) {
		t.Fatalf("disabled version error = %v", err)
	}
}

func TestProfileRegistrySupportsSelectableActiveVersion(t *testing.T) {
	registry, err := LoadProfileRegistry(repositoryPromptDir(t))
	if err != nil {
		t.Fatalf("LoadProfileRegistry() error = %v", err)
	}
	profile, err := registry.ResolveVersion("knowledge-guide", "v1", "user-1")
	if err != nil {
		t.Fatalf("ResolveVersion() error = %v", err)
	}
	if profile.Version != "v1" || registry.ActiveVersion("knowledge-guide") != "v1" {
		t.Fatalf("active version mismatch: %+v", profile)
	}
}

func TestProfileRegistryNormalizesSlugAndAppliesPilotGateToPinnedVersion(t *testing.T) {
	t.Setenv("PI_KNOWLEDGE_GUIDE_PILOT_USERS", "user-1")
	registry, err := LoadProfileRegistry(repositoryPromptDir(t))
	if err != nil {
		t.Fatalf("LoadProfileRegistry() error = %v", err)
	}
	if _, err := registry.Resolve(" knowledge-guide ", "user-1"); err != nil {
		t.Fatalf("Resolve() with surrounding whitespace error = %v", err)
	}
	if _, err := registry.ResolveVersion("knowledge-guide", "v1", "other-user"); !errors.Is(err, ErrProfileNotAvailable) {
		t.Fatalf("ResolveVersion() pilot restriction error = %v", err)
	}
}

func TestLoadProfileRegistryRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{name: "permission", key: "PI_KNOWLEDGE_GUIDE_PERMISSION_DEFAULT", value: "sometimes"},
		{name: "enabled", key: "PI_KNOWLEDGE_GUIDE_ENABLED", value: "perhaps"},
		{name: "tool limit", key: "PI_KNOWLEDGE_GUIDE_MAX_TOOL_CALLS", value: "0"},
		{name: "active version", key: "PI_KNOWLEDGE_GUIDE_ACTIVE_VERSION", value: "v99"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.key, tt.value)
			if _, err := LoadProfileRegistry(repositoryPromptDir(t)); err == nil {
				t.Fatal("LoadProfileRegistry() error = nil")
			}
		})
	}
}
