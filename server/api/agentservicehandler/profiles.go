package agentservicehandler

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	PermissionAsk  = "ask"
	PermissionAuto = "auto"
)

var (
	ErrProfileNotFound     = errors.New("agentic service profile not found")
	ErrProfileDisabled     = errors.New("agentic service profile is disabled")
	ErrProfileNotAvailable = errors.New("agentic service profile is not available to this user")
)

type ProfileLimits struct {
	MaxToolCalls     int           `json:"max_tool_calls"`
	MaxElapsed       time.Duration `json:"max_elapsed"`
	MaxOutputTokens  int           `json:"max_output_tokens"`
	MaxEvidenceBytes int           `json:"max_evidence_bytes"`
}

type PiProfile struct {
	Slug                       string        `json:"slug"`
	Version                    string        `json:"version"`
	FriendlyName               string        `json:"friendly_name"`
	Description                string        `json:"description"`
	Provider                   string        `json:"provider"`
	Model                      string        `json:"model"`
	ProviderDisclosure         string        `json:"provider_disclosure"`
	ThinkingLevel              string        `json:"thinking_level"`
	PromptFile                 string        `json:"prompt_file"`
	SystemPrompt               string        `json:"-"`
	AllowedTools               []string      `json:"allowed_tools"`
	AllowedKnowledgeStores     []string      `json:"allowed_knowledge_stores"`
	AllowedDocumentGroups      []string      `json:"allowed_document_groups"`
	PermissionDefault          string        `json:"permission_default"`
	Limits                     ProfileLimits `json:"limits"`
	Enabled                    bool          `json:"enabled"`
	PilotUsers                 []string      `json:"-"`
	SaveAndResumeConversations bool          `json:"save_and_resume_conversations"`
}

type ProfileRegistry struct {
	versions map[string]map[string]PiProfile
	active   map[string]string
}

type profileDefinition struct {
	slug         string
	envPrefix    string
	friendlyName string
	description  string
	promptFile   string
	provider     string
	model        string
	thinking     string
	stores       []string
	limits       ProfileLimits
}

var knowledgeToolNames = []string{
	"search_knowledge",
	"read_source_passages",
	"get_artifact_details",
	"get_document_context",
	"find_related_knowledge",
}

func LoadProfileRegistry(promptDir string) (*ProfileRegistry, error) {
	definitions := []profileDefinition{
		{
			slug: "knowledge-guide", envPrefix: "PI_KNOWLEDGE_GUIDE", friendlyName: "Knowledge Guide",
			description: "Answers questions using evidence from ChenWeb's knowledge base.",
			promptFile:  "prompt-agent-knowledge-guide-v1.md", provider: "anthropic", model: "claude-sonnet-4-5",
			thinking: "medium", stores: []string{"Research"},
			limits: ProfileLimits{MaxToolCalls: 12, MaxElapsed: 90 * time.Second, MaxOutputTokens: 1200, MaxEvidenceBytes: 65536},
		},
		{
			slug: "problem-diagnostics", envPrefix: "PI_PROBLEM_DIAGNOSTICS", friendlyName: "Problem Diagnosis Guide",
			description: "Investigates product, process, and documentation problems using read-only evidence.",
			promptFile:  "prompt-agent-problem-diagnosis-v1.md", provider: "anthropic", model: "claude-sonnet-4-5",
			thinking: "high", stores: []string{"Research"},
			limits: ProfileLimits{MaxToolCalls: 16, MaxElapsed: 120 * time.Second, MaxOutputTokens: 1600, MaxEvidenceBytes: 98304},
		},
	}

	registry := &ProfileRegistry{
		versions: make(map[string]map[string]PiProfile, len(definitions)),
		active:   make(map[string]string, len(definitions)),
	}
	for _, definition := range definitions {
		profile, err := loadProfile(promptDir, definition)
		if err != nil {
			return nil, fmt.Errorf("load profile %q: %w", definition.slug, err)
		}
		registry.versions[profile.Slug] = map[string]PiProfile{profile.Version: profile}
		activeVersion := envDefault(definition.envPrefix+"_ACTIVE_VERSION", "v1")
		if _, ok := registry.versions[profile.Slug][activeVersion]; !ok {
			return nil, fmt.Errorf("profile %q active version %q is unavailable", profile.Slug, activeVersion)
		}
		registry.active[profile.Slug] = activeVersion
	}
	return registry, nil
}

func loadProfile(promptDir string, definition profileDefinition) (PiProfile, error) {
	promptPath := filepath.Join(promptDir, definition.promptFile)
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		return PiProfile{}, fmt.Errorf("read prompt %q: %w", promptPath, err)
	}
	if strings.TrimSpace(string(promptBytes)) == "" {
		return PiProfile{}, fmt.Errorf("prompt %q is empty", promptPath)
	}

	enabled, err := envBool(definition.envPrefix+"_ENABLED", true)
	if err != nil {
		return PiProfile{}, err
	}
	permission := strings.ToLower(envDefault(definition.envPrefix+"_PERMISSION_DEFAULT", PermissionAuto))
	if permission != PermissionAsk && permission != PermissionAuto {
		return PiProfile{}, fmt.Errorf("%s_PERMISSION_DEFAULT must be ask or auto", definition.envPrefix)
	}
	maxToolCalls, err := envPositiveInt(definition.envPrefix+"_MAX_TOOL_CALLS", definition.limits.MaxToolCalls)
	if err != nil {
		return PiProfile{}, err
	}
	maxElapsedSeconds, err := envPositiveInt(definition.envPrefix+"_MAX_ELAPSED_SECONDS", int(definition.limits.MaxElapsed/time.Second))
	if err != nil {
		return PiProfile{}, err
	}
	maxOutputTokens, err := envPositiveInt(definition.envPrefix+"_MAX_OUTPUT_TOKENS", definition.limits.MaxOutputTokens)
	if err != nil {
		return PiProfile{}, err
	}
	maxEvidenceBytes, err := envPositiveInt(definition.envPrefix+"_MAX_EVIDENCE_BYTES", definition.limits.MaxEvidenceBytes)
	if err != nil {
		return PiProfile{}, err
	}

	provider := envDefault(definition.envPrefix+"_PROVIDER", definition.provider)
	model := envDefault(definition.envPrefix+"_MODEL", definition.model)
	stores := envCSV(definition.envPrefix+"_ALLOWED_STORES", definition.stores)
	if len(stores) == 0 {
		return PiProfile{}, fmt.Errorf("%s_ALLOWED_STORES must contain at least one store", definition.envPrefix)
	}
	return PiProfile{
		Slug: definition.slug, Version: "v1", FriendlyName: definition.friendlyName,
		Description: definition.description, Provider: provider, Model: model,
		ProviderDisclosure: envDefault(definition.envPrefix+"_PROVIDER_DISCLOSURE", "Messages and retrieved evidence are sent to the "+provider+" model provider."),
		ThinkingLevel:      envDefault(definition.envPrefix+"_THINKING_LEVEL", definition.thinking),
		PromptFile:         definition.promptFile, SystemPrompt: strings.TrimSpace(string(promptBytes)),
		AllowedTools: append([]string(nil), knowledgeToolNames...), AllowedKnowledgeStores: stores,
		AllowedDocumentGroups: envCSV(definition.envPrefix+"_ALLOWED_DOCUMENT_GROUPS", nil),
		PermissionDefault:     permission,
		Limits:                ProfileLimits{MaxToolCalls: maxToolCalls, MaxElapsed: time.Duration(maxElapsedSeconds) * time.Second, MaxOutputTokens: maxOutputTokens, MaxEvidenceBytes: maxEvidenceBytes},
		Enabled:               enabled, PilotUsers: envCSV(definition.envPrefix+"_PILOT_USERS", nil),
		SaveAndResumeConversations: true,
	}, nil
}

func (r *ProfileRegistry) Resolve(slug, userID string) (PiProfile, error) {
	slug = strings.TrimSpace(slug)
	version, ok := r.active[slug]
	if !ok {
		return PiProfile{}, ErrProfileNotFound
	}
	profile := r.versions[slug][version]
	if !profile.Enabled {
		return PiProfile{}, ErrProfileDisabled
	}
	if len(profile.PilotUsers) > 0 && !contains(profile.PilotUsers, strings.TrimSpace(userID)) {
		return PiProfile{}, ErrProfileNotAvailable
	}
	return cloneProfile(profile), nil
}

func (r *ProfileRegistry) ResolveVersion(slug, version, userID string) (PiProfile, error) {
	versions, ok := r.versions[strings.TrimSpace(slug)]
	if !ok {
		return PiProfile{}, ErrProfileNotFound
	}
	profile, ok := versions[strings.TrimSpace(version)]
	if !ok {
		return PiProfile{}, ErrProfileNotFound
	}
	if !profile.Enabled {
		return PiProfile{}, ErrProfileDisabled
	}
	if len(profile.PilotUsers) > 0 && !contains(profile.PilotUsers, strings.TrimSpace(userID)) {
		return PiProfile{}, ErrProfileNotAvailable
	}
	return cloneProfile(profile), nil
}

func (r *ProfileRegistry) ActiveVersion(slug string) string {
	return r.active[strings.TrimSpace(slug)]
}

func cloneProfile(profile PiProfile) PiProfile {
	profile.AllowedTools = append([]string(nil), profile.AllowedTools...)
	profile.AllowedKnowledgeStores = append([]string(nil), profile.AllowedKnowledgeStores...)
	profile.AllowedDocumentGroups = append([]string(nil), profile.AllowedDocumentGroups...)
	profile.PilotUsers = append([]string(nil), profile.PilotUsers...)
	return profile
}

func envDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", key)
	}
	return value, nil
}

func envPositiveInt(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", key)
	}
	return value, nil
}

func envCSV(key string, fallback []string) []string {
	raw, ok := os.LookupEnv(key)
	if !ok {
		return append([]string(nil), fallback...)
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
