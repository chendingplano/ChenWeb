package agentservicehandler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	sharedllm "github.com/chendingplano/shared/go/api/llm"
)

type AgentRunProfile struct {
	Slug              string   `json:"slug"`
	Version           string   `json:"version"`
	Provider          string   `json:"provider"`
	Model             string   `json:"model"`
	SystemPrompt      string   `json:"systemPrompt"`
	ThinkingLevel     string   `json:"thinkingLevel"`
	PermissionDefault string   `json:"permissionDefault"`
	AllowedTools      []string `json:"allowedTools"`
	MaxToolCalls      int      `json:"maxToolCalls"`
	MaxElapsedMs      int64    `json:"maxElapsedMs"`
	MaxOutputTokens   int      `json:"maxOutputTokens"`
	MaxEvidenceBytes  int      `json:"maxEvidenceBytes"`
}

func MapAgentProfile(profile AgentServiceProfile, permission string) AgentRunProfile {
	return AgentRunProfile{Slug: profile.Slug, Version: profile.Version, Provider: profile.Provider, Model: profile.Model, SystemPrompt: profile.SystemPrompt, ThinkingLevel: profile.ThinkingLevel, PermissionDefault: permission, AllowedTools: append([]string(nil), profile.AllowedTools...), MaxToolCalls: profile.Limits.MaxToolCalls, MaxElapsedMs: profile.Limits.MaxElapsed.Milliseconds(), MaxOutputTokens: profile.Limits.MaxOutputTokens, MaxEvidenceBytes: profile.Limits.MaxEvidenceBytes}
}

type AgentHistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ToolCalls are an assistant turn's earlier tool calls, replayed by the
	// adapter as tool-call and tool-result messages before the answer.
	ToolCalls []AgentHistoryToolCall `json:"toolCalls,omitempty"`
}

type AgentHistoryToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Result    string          `json:"result"`
	IsError   bool            `json:"isError"`
}
type AgentRunRequest struct {
	RunID          string                 `json:"runId"`
	ConversationID string                 `json:"conversationId"`
	UserID         string                 `json:"userId"`
	Message        string                 `json:"message"`
	Capability     string                 `json:"capability"`
	Profile        AgentRunProfile        `json:"profile"`
	History        []AgentHistoryMessage  `json:"history"`
	Knowledge      *AgentKnowledgeContext `json:"knowledge,omitempty"`
	HistorySummary string                 `json:"historySummary,omitempty"`
}

// AgentSummaryRequest folds earlier turns into a conversation's rolling
// summary with the conversation's own model.
type AgentSummaryRequest struct {
	UserID          string
	Provider        string
	Model           string
	SystemPrompt    string
	PreviousSummary string
	Messages        []AgentHistoryMessage
	MaxTokens       int
}

// modelInfoTTL bounds how long a model's context window is cached.
const modelInfoTTL = 10 * time.Minute

type cachedModelInfo struct {
	contextWindow int
	expires       time.Time
}

type PiHarnessAdapter struct {
	client *sharedllm.PiGatewayClient
	mu     sync.Mutex
	models map[string]cachedModelInfo
}

// piAdapterVersion identifies the Pi gateway protocol this adapter speaks.
const piAdapterVersion = "1"

// piMaxRequestBytes leaves headroom under the gateway's 512 KiB body limit.
const piMaxRequestBytes = 480 * 1024

func (g *PiHarnessAdapter) Describe() HarnessDescriptor {
	return HarnessDescriptor{Name: "pi", AdapterVersion: piAdapterVersion, MaxRequestBytes: piMaxRequestBytes,
		Capabilities: []HarnessCapability{CapabilityStreaming, CapabilityCancellation, CapabilityInteractiveToolApproval,
			CapabilityUsageReporting, CapabilityHistoryReplay, CapabilityContextCapacityReporting, CapabilityHistorySummary}}
}

func NewPiHarnessAdapter(rawURL, secret string, client *http.Client) *PiHarnessAdapter {
	return &PiHarnessAdapter{client: sharedllm.NewPiGatewayClient(rawURL, secret, client), models: make(map[string]cachedModelInfo)}
}
func (g *PiHarnessAdapter) available() bool {
	return g != nil && g.client != nil
}
func (g *PiHarnessAdapter) endpoint(path string) string {
	if g == nil || g.client == nil {
		return ""
	}
	// Preserve the historical test-only helper without owning gateway HTTP.
	u, _ := url.Parse("http://127.0.0.1:4317")
	if raw := g.client.BaseURL(); raw != "" {
		u, _ = url.Parse(raw)
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = ""
	return u.String()
}
func (g *PiHarnessAdapter) Start(ctx context.Context, run AgentRunRequest) (io.ReadCloser, error) {
	gatewayRun := sharedllm.PiGatewayRun{RunID: run.RunID, ConversationID: run.ConversationID, UserID: run.UserID, Message: run.Message, Capability: run.Capability, Profile: run.Profile, History: run.History, HistorySummary: run.HistorySummary, Capture: &sharedllm.RequestCapture{UserID: run.UserID}}
	if run.Knowledge != nil {
		// A nil pointer inside the interface would still be sent as null.
		gatewayRun.Knowledge = run.Knowledge
	}
	return g.client.Start(ctx, gatewayRun)
}
func (g *PiHarnessAdapter) Cancel(ctx context.Context, runID string) error {
	return g.client.Cancel(ctx, runID)
}
func (g *PiHarnessAdapter) Decide(ctx context.Context, runID, requestID string, allowed bool) error {
	return g.client.Decide(ctx, runID, requestID, allowed)
}

// ModelContextWindow returns the model's context window as reported by the
// Pi gateway, cached for modelInfoTTL.
func (g *PiHarnessAdapter) ModelContextWindow(ctx context.Context, provider, model string) (int, error) {
	key := provider + "/" + model
	g.mu.Lock()
	cached, ok := g.models[key]
	g.mu.Unlock()
	if ok && time.Now().Before(cached.expires) {
		return cached.contextWindow, nil
	}
	info, err := g.client.ModelInfo(ctx, provider, model)
	if err != nil {
		return 0, err
	}
	g.mu.Lock()
	g.models[key] = cachedModelInfo{contextWindow: info.ContextWindow, expires: time.Now().Add(modelInfoTTL)}
	g.mu.Unlock()
	return info.ContextWindow, nil
}

func (g *PiHarnessAdapter) Summarize(ctx context.Context, in AgentSummaryRequest) (string, error) {
	messages := make([]sharedllm.PiGatewayMessage, 0, len(in.Messages))
	for _, message := range in.Messages {
		messages = append(messages, sharedllm.PiGatewayMessage{Role: message.Role, Content: message.Content})
	}
	return g.client.Summarize(ctx, sharedllm.PiGatewaySummaryRequest{UserID: in.UserID, Provider: in.Provider, Model: in.Model, SystemPrompt: in.SystemPrompt,
		PreviousSummary: in.PreviousSummary, Messages: messages, MaxTokens: in.MaxTokens, Capture: &sharedllm.RequestCapture{UserID: in.UserID}})
}
