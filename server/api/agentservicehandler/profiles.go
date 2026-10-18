package agentservicehandler

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"text/template"
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
	// MaxHistoryTokens caps the estimated tokens of earlier turns sent per run.
	MaxHistoryTokens int `json:"max_history_tokens"`
	// MaxHistoryToolResultBytes is the largest tool result carried unshortened
	// in later turns' history.
	MaxHistoryToolResultBytes int `json:"max_history_tool_result_bytes"`
}

// defaultMaxHistoryTokens keeps history affordable on models whose context
// window is very large.
const defaultMaxHistoryTokens = 32000

const (
	defaultMaxHistoryToolResultBytes = 4096
	maxHistoryToolResultBytesCeiling = 32768
	// defaultSnapshotHours is how long saved history is shown as saved,
	// regardless of later access or content changes.
	defaultSnapshotHours = 48
)

type AgentServiceProfile struct {
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
	versions          map[string]map[string]AgentServiceProfile
	active            map[string]string
	knowledgeContext  *template.Template
	versionStore      ProfileVersionStore
	versionStoreReady atomic.Bool
	// historySummaryPrompt is the system prompt for folding earlier turns.
	historySummaryPrompt string
	// snapshotWindow is PI_HISTORY_SNAPSHOT_DUR: history younger than this is
	// shown and resent as saved.
	snapshotWindow time.Duration
}

// WithVersionStore attaches durable profile storage. The registry keeps using
// its environment-backed profiles until EnsureVersionStoreSeeded succeeds.
func (r *ProfileRegistry) WithVersionStore(store ProfileVersionStore) *ProfileRegistry {
	r.versionStore = store
	return r
}

func (r *ProfileRegistry) EnsureVersionStoreSeeded(ctx context.Context) error {
	if r.versionStore == nil {
		return errors.New("profile version store is unavailable")
	}
	if err := r.versionStore.EnsureSeeded(ctx, r); err != nil {
		return err
	}
	r.versionStoreReady.Store(true)
	return nil
}

func (r *ProfileRegistry) VersionStoreReady() bool {
	return r != nil && r.versionStore != nil && r.versionStoreReady.Load()
}

// ResolveActive loads the active profile from durable storage after initial
// seeding; before that it retains the environment-backed startup behavior.
func (r *ProfileRegistry) ResolveActive(ctx context.Context, slug, userID string) (AgentServiceProfile, error) {
	if !r.VersionStoreReady() {
		return r.Resolve(slug, userID)
	}
	profiles, err := r.versionStore.ListActive(ctx)
	if err != nil {
		return AgentServiceProfile{}, err
	}
	for _, profile := range profiles {
		if profile.Slug == strings.TrimSpace(slug) {
			return authorizeProfile(profile, userID)
		}
	}
	return AgentServiceProfile{}, ErrProfileNotFound
}

// ResolvePinned loads the exact immutable profile version recorded on a
// conversation after initial seeding.
func (r *ProfileRegistry) ResolvePinned(ctx context.Context, slug, version, userID string) (AgentServiceProfile, error) {
	if !r.VersionStoreReady() {
		return r.ResolveVersion(slug, version, userID)
	}
	profile, err := r.versionStore.LoadVersion(ctx, strings.TrimSpace(slug), strings.TrimSpace(version))
	if err != nil {
		return AgentServiceProfile{}, err
	}
	return authorizeProfile(profile, userID)
}

func authorizeProfile(profile AgentServiceProfile, userID string) (AgentServiceProfile, error) {
	if !profile.Enabled {
		return AgentServiceProfile{}, ErrProfileDisabled
	}
	if len(profile.PilotUsers) > 0 && !contains(profile.PilotUsers, strings.TrimSpace(userID)) {
		return AgentServiceProfile{}, ErrProfileNotAvailable
	}
	return cloneProfile(profile), nil
}

func isProfileAvailabilityError(err error) bool {
	return errors.Is(err, ErrProfileNotFound) || errors.Is(err, ErrProfileDisabled) || errors.Is(err, ErrProfileNotAvailable)
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

// savedToolResultTool returns the full stored result of an earlier tool call
// in the same conversation. It reads the conversation, not a knowledge store.
const savedToolResultTool = "get_saved_tool_result"

// agentToolNames is every tool a run may offer Pi.
var agentToolNames = append(append([]string(nil), knowledgeToolNames...), savedToolResultTool)

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

	knowledgeContext, err := loadKnowledgeContextTemplate(promptDir)
	if err != nil {
		return nil, err
	}
	historySummaryPrompt, err := loadHistorySummaryPrompt(promptDir)
	if err != nil {
		return nil, err
	}
	snapshotHours, err := envPositiveInt("PI_HISTORY_SNAPSHOT_DUR", defaultSnapshotHours)
	if err != nil {
		return nil, err
	}
	registry := &ProfileRegistry{
		historySummaryPrompt: historySummaryPrompt,
		snapshotWindow:       time.Duration(snapshotHours) * time.Hour,
		versions:             make(map[string]map[string]AgentServiceProfile, len(definitions)),
		active:               make(map[string]string, len(definitions)),
		knowledgeContext:     knowledgeContext,
	}
	for _, definition := range definitions {
		profile, err := loadProfile(promptDir, definition)
		if err != nil {
			return nil, fmt.Errorf("load profile %q: %w", definition.slug, err)
		}
		registry.versions[profile.Slug] = map[string]AgentServiceProfile{profile.Version: profile}
		activeVersion := envDefault(definition.envPrefix+"_ACTIVE_VERSION", "v1")
		if _, ok := registry.versions[profile.Slug][activeVersion]; !ok {
			return nil, fmt.Errorf("profile %q active version %q is unavailable", profile.Slug, activeVersion)
		}
		registry.active[profile.Slug] = activeVersion
	}
	return registry, nil
}

func loadProfile(promptDir string, definition profileDefinition) (AgentServiceProfile, error) {
	promptPath := filepath.Join(promptDir, definition.promptFile)
	promptBytes, err := os.ReadFile(promptPath)
	if err != nil {
		return AgentServiceProfile{}, fmt.Errorf("read prompt %q: %w", promptPath, err)
	}
	if strings.TrimSpace(string(promptBytes)) == "" {
		return AgentServiceProfile{}, fmt.Errorf("prompt %q is empty", promptPath)
	}

	enabled, err := envBool(definition.envPrefix+"_ENABLED", true)
	if err != nil {
		return AgentServiceProfile{}, err
	}
	permission := strings.ToLower(envDefault(definition.envPrefix+"_PERMISSION_DEFAULT", PermissionAuto))
	if permission != PermissionAsk && permission != PermissionAuto {
		return AgentServiceProfile{}, fmt.Errorf("%s_PERMISSION_DEFAULT must be ask or auto", definition.envPrefix)
	}
	maxToolCalls, err := envPositiveInt(definition.envPrefix+"_MAX_TOOL_CALLS", definition.limits.MaxToolCalls)
	if err != nil {
		return AgentServiceProfile{}, err
	}
	maxElapsedSeconds, err := envPositiveInt(definition.envPrefix+"_MAX_ELAPSED_SECONDS", int(definition.limits.MaxElapsed/time.Second))
	if err != nil {
		return AgentServiceProfile{}, err
	}
	maxOutputTokens, err := envPositiveInt(definition.envPrefix+"_MAX_OUTPUT_TOKENS", definition.limits.MaxOutputTokens)
	if err != nil {
		return AgentServiceProfile{}, err
	}
	maxEvidenceBytes, err := envPositiveInt(definition.envPrefix+"_MAX_EVIDENCE_BYTES", definition.limits.MaxEvidenceBytes)
	if err != nil {
		return AgentServiceProfile{}, err
	}
	maxHistoryTokens, err := envPositiveInt(definition.envPrefix+"_MAX_HISTORY_TOKENS", defaultMaxHistoryTokens)
	if err != nil {
		return AgentServiceProfile{}, err
	}
	maxHistoryToolResultBytes, err := envPositiveInt(definition.envPrefix+"_MAX_HISTORY_TOOL_RESULT_BYTES", defaultMaxHistoryToolResultBytes)
	if err != nil {
		return AgentServiceProfile{}, err
	}
	if maxHistoryToolResultBytes > maxHistoryToolResultBytesCeiling {
		return AgentServiceProfile{}, fmt.Errorf("%s_MAX_HISTORY_TOOL_RESULT_BYTES must be %d or less", definition.envPrefix, maxHistoryToolResultBytesCeiling)
	}

	provider := envDefault(definition.envPrefix+"_PROVIDER", definition.provider)
	model := envDefault(definition.envPrefix+"_MODEL", definition.model)
	stores := envCSV(definition.envPrefix+"_ALLOWED_STORES", definition.stores)
	if len(stores) == 0 {
		return AgentServiceProfile{}, fmt.Errorf("%s_ALLOWED_STORES must contain at least one store", definition.envPrefix)
	}
	return AgentServiceProfile{
		Slug: definition.slug, Version: "v1", FriendlyName: definition.friendlyName,
		Description: definition.description, Provider: provider, Model: model,
		ProviderDisclosure: envDefault(definition.envPrefix+"_PROVIDER_DISCLOSURE", "Messages and retrieved evidence are sent to the "+provider+" model provider."),
		ThinkingLevel:      envDefault(definition.envPrefix+"_THINKING_LEVEL", definition.thinking),
		PromptFile:         definition.promptFile, SystemPrompt: strings.TrimSpace(string(promptBytes)),
		AllowedTools: append([]string(nil), knowledgeToolNames...), AllowedKnowledgeStores: stores,
		AllowedDocumentGroups: envCSV(definition.envPrefix+"_ALLOWED_DOCUMENT_GROUPS", nil),
		PermissionDefault:     permission,
		Limits:                ProfileLimits{MaxToolCalls: maxToolCalls, MaxElapsed: time.Duration(maxElapsedSeconds) * time.Second, MaxOutputTokens: maxOutputTokens, MaxEvidenceBytes: maxEvidenceBytes, MaxHistoryTokens: maxHistoryTokens, MaxHistoryToolResultBytes: maxHistoryToolResultBytes},
		Enabled:               enabled, PilotUsers: envCSV(definition.envPrefix+"_PILOT_USERS", nil),
		SaveAndResumeConversations: true,
	}, nil
}

func (r *ProfileRegistry) Resolve(slug, userID string) (AgentServiceProfile, error) {
	slug = strings.TrimSpace(slug)
	version, ok := r.active[slug]
	if !ok {
		return AgentServiceProfile{}, ErrProfileNotFound
	}
	profile := r.versions[slug][version]
	return authorizeProfile(profile, userID)
}

func (r *ProfileRegistry) ResolveVersion(slug, version, userID string) (AgentServiceProfile, error) {
	versions, ok := r.versions[strings.TrimSpace(slug)]
	if !ok {
		return AgentServiceProfile{}, ErrProfileNotFound
	}
	profile, ok := versions[strings.TrimSpace(version)]
	if !ok {
		return AgentServiceProfile{}, ErrProfileNotFound
	}
	return authorizeProfile(profile, userID)
}

func (r *ProfileRegistry) ActiveVersion(slug string) string {
	return r.active[strings.TrimSpace(slug)]
}

func cloneProfile(profile AgentServiceProfile) AgentServiceProfile {
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
