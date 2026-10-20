package agentservicehandler

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrInvalidProfileCandidate = errors.New("invalid agentic service profile candidate")

// ProfileCandidate is the editable, untrusted input used to create a new
// profile version. Tools, version, prompt text, and provider disclosure are
// server-owned and therefore cannot be supplied here.
type ProfileCandidate struct {
	Slug                   string        `json:"slug"`
	FriendlyName           string        `json:"friendly_name"`
	Description            string        `json:"description"`
	Provider               string        `json:"provider"`
	Model                  string        `json:"model"`
	ThinkingLevel          string        `json:"thinking_level"`
	PromptFile             string        `json:"prompt_file"`
	AllowedKnowledgeStores []string      `json:"allowed_knowledge_stores"`
	AllowedDocumentGroups  []string      `json:"allowed_document_groups"`
	PermissionDefault      string        `json:"permission_default"`
	Limits                 ProfileLimits `json:"limits"`
	Enabled                bool          `json:"enabled"`
	PilotUsers             []string      `json:"pilot_users"`
}

type ValidatedProfile struct {
	Profile    AgentServiceProfile `json:"profile"`
	PromptRef  string              `json:"prompt_ref"`
	PromptHash string              `json:"prompt_hash"`
}

// ValidateProfileCandidate validates an admin candidate and snapshots its
// selected prompt from promptDir. Caller-provided prompt text is never used.
func ValidateProfileCandidate(promptDir string, candidate ProfileCandidate) (ValidatedProfile, error) {
	invalid := func(format string, args ...any) (ValidatedProfile, error) {
		return ValidatedProfile{}, fmt.Errorf("%w: %s", ErrInvalidProfileCandidate, fmt.Sprintf(format, args...))
	}

	candidate.Slug = strings.TrimSpace(candidate.Slug)
	if _, ok := profileSlugSet[candidate.Slug]; !ok {
		return invalid("unsupported service slug %q", candidate.Slug)
	}
	candidate.Provider = strings.TrimSpace(candidate.Provider)
	candidate.Model = strings.TrimSpace(candidate.Model)
	if candidate.Provider == "" || candidate.Model == "" {
		return invalid("provider and model are required")
	}
	if candidate.PermissionDefault != PermissionAsk && candidate.PermissionDefault != PermissionAuto {
		return invalid("permission_default must be ask or auto")
	}
	switch candidate.ThinkingLevel {
	case "off", "minimal", "low", "medium", "high", "xhigh":
	default:
		return invalid("unsupported thinking_level %q", candidate.ThinkingLevel)
	}
	candidate.AllowedKnowledgeStores = cleanNonEmptyStrings(candidate.AllowedKnowledgeStores)
	if len(candidate.AllowedKnowledgeStores) == 0 {
		return invalid("at least one allowed knowledge store is required")
	}
	groups, err := cleanDocumentGroups(candidate.AllowedDocumentGroups)
	if err != nil {
		return invalid("%v", err)
	}
	candidate.AllowedDocumentGroups = groups
	candidate.PilotUsers = normalizePilotUsers(candidate.PilotUsers)
	if err := validateProfileLimits(candidate.Limits); err != nil {
		return invalid("%v", err)
	}
	ref := strings.TrimSpace(candidate.PromptFile)
	allowedPrompt := map[string]string{
		"knowledge-guide":     "prompt-agent-knowledge-guide-v1.md",
		"problem-diagnostics": "prompt-agent-problem-diagnosis-v1.md",
	}[candidate.Slug]
	if ref == "" || filepath.Base(ref) != ref || strings.ContainsAny(ref, `/\\`) || ref != allowedPrompt {
		return invalid("prompt_file is not approved for service %q", candidate.Slug)
	}
	path := filepath.Join(promptDir, ref)
	lstat, err := os.Lstat(path)
	if err != nil {
		return invalid("inspect prompt %q: %v", ref, err)
	}
	if !lstat.Mode().IsRegular() {
		return invalid("prompt %q must be a regular file, not a symlink", ref)
	}
	file, err := os.Open(path)
	if err != nil {
		return invalid("open prompt %q: %v", ref, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(lstat, info) {
		return invalid("prompt %q must be a regular file", ref)
	}
	content, err := io.ReadAll(file)
	if err != nil {
		return invalid("read prompt %q: %v", ref, err)
	}
	if strings.TrimSpace(string(content)) == "" {
		return invalid("prompt %q is empty", ref)
	}
	if !utf8.Valid(content) || strings.ContainsRune(string(content), '\x00') {
		return invalid("prompt %q must be valid UTF-8 text", ref)
	}
	hash := sha256.Sum256(content)
	profile := AgentServiceProfile{
		Slug: candidate.Slug, FriendlyName: strings.TrimSpace(candidate.FriendlyName),
		Description: strings.TrimSpace(candidate.Description), Provider: candidate.Provider,
		Model: candidate.Model, ProviderDisclosure: "Messages and retrieved evidence are sent to the " + candidate.Provider + " model provider.",
		ThinkingLevel: candidate.ThinkingLevel, PromptFile: ref, SystemPrompt: string(content),
		AllowedTools: append([]string(nil), knowledgeToolNames...), AllowedKnowledgeStores: candidate.AllowedKnowledgeStores,
		AllowedDocumentGroups: candidate.AllowedDocumentGroups, PermissionDefault: candidate.PermissionDefault,
		Limits: candidate.Limits, Enabled: candidate.Enabled, PilotUsers: candidate.PilotUsers,
		SaveAndResumeConversations: true, Harness: DefaultHarness,
	}
	return ValidatedProfile{Profile: profile, PromptRef: ref, PromptHash: hex.EncodeToString(hash[:])}, nil
}

func validateProfileLimits(l ProfileLimits) error {
	checks := []struct {
		name       string
		value, max int
	}{
		{"max_tool_calls", l.MaxToolCalls, 64}, {"max_output_tokens", l.MaxOutputTokens, 8192},
		{"max_evidence_bytes", l.MaxEvidenceBytes, 1_048_576}, {"max_history_tokens", l.MaxHistoryTokens, 128_000},
		{"max_history_tool_result_bytes", l.MaxHistoryToolResultBytes, 32_768},
	}
	for _, check := range checks {
		if check.value < 1 || check.value > check.max {
			return fmt.Errorf("%s must be between 1 and %d", check.name, check.max)
		}
	}
	seconds := int(l.MaxElapsed / time.Second)
	if l.MaxElapsed%time.Second != 0 || seconds < 1 || seconds > 600 {
		return errors.New("max_elapsed must be a whole number of seconds between 1 and 600")
	}
	return nil
}

func cleanNonEmptyStrings(values []string) []string {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			cleaned = append(cleaned, value)
		}
	}
	return cleaned
}

func cleanDocumentGroups(values []string) ([]string, error) {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, errors.New("document group values must be non-empty strings")
		}
		cleaned = append(cleaned, value)
	}
	return cleaned, nil
}

func normalizePilotUsers(values []string) []string {
	cleaned := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}
