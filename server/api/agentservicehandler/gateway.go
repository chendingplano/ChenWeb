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

type GatewayRunProfile struct {
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

func MapGatewayProfile(profile PiProfile, permission string) GatewayRunProfile {
	return GatewayRunProfile{Slug: profile.Slug, Version: profile.Version, Provider: profile.Provider, Model: profile.Model, SystemPrompt: profile.SystemPrompt, ThinkingLevel: profile.ThinkingLevel, PermissionDefault: permission, AllowedTools: append([]string(nil), profile.AllowedTools...), MaxToolCalls: profile.Limits.MaxToolCalls, MaxElapsedMs: profile.Limits.MaxElapsed.Milliseconds(), MaxOutputTokens: profile.Limits.MaxOutputTokens, MaxEvidenceBytes: profile.Limits.MaxEvidenceBytes}
}

type GatewayHistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ToolCalls are an assistant turn's earlier tool calls, replayed by the
	// gateway as tool-call and tool-result messages before the answer.
	ToolCalls []GatewayHistoryToolCall `json:"toolCalls,omitempty"`
}

type GatewayHistoryToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Result    string          `json:"result"`
	IsError   bool            `json:"isError"`
}
type GatewayRunRequest struct {
	RunID          string                   `json:"runId"`
	ConversationID string                   `json:"conversationId"`
	UserID         string                   `json:"userId"`
	Message        string                   `json:"message"`
	Capability     string                   `json:"capability"`
	Profile        GatewayRunProfile        `json:"profile"`
	History        []GatewayHistoryMessage  `json:"history"`
	Knowledge      *GatewayKnowledgeContext `json:"knowledge,omitempty"`
	HistorySummary string                   `json:"historySummary,omitempty"`
}

// GatewaySummaryRequest folds earlier turns into a conversation's rolling
// summary with the conversation's own model.
type GatewaySummaryRequest struct {
	UserID          string
	Provider        string
	Model           string
	SystemPrompt    string
	PreviousSummary string
	Messages        []GatewayHistoryMessage
	MaxTokens       int
}

// modelInfoTTL bounds how long a model's context window is cached.
const modelInfoTTL = 10 * time.Minute

type cachedModelInfo struct {
	contextWindow int
	expires       time.Time
}

type PiGatewayClient struct {
	client *sharedllm.PiGatewayClient
	mu     sync.Mutex
	models map[string]cachedModelInfo
}

func NewPiGatewayClient(rawURL, secret string, client *http.Client) *PiGatewayClient {
	return &PiGatewayClient{client: sharedllm.NewPiGatewayClient(rawURL, secret, client), models: make(map[string]cachedModelInfo)}
}
func (g *PiGatewayClient) available() bool {
	return g != nil && g.client != nil
}
func (g *PiGatewayClient) endpoint(path string) string {
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
func (g *PiGatewayClient) Start(ctx context.Context, run GatewayRunRequest) (io.ReadCloser, error) {
	gatewayRun := sharedllm.PiGatewayRun{RunID: run.RunID, ConversationID: run.ConversationID, UserID: run.UserID, Message: run.Message, Capability: run.Capability, Profile: run.Profile, History: run.History, HistorySummary: run.HistorySummary, Capture: &sharedllm.RequestCapture{UserID: run.UserID}}
	if run.Knowledge != nil {
		// A nil pointer inside the interface would still be sent as null.
		gatewayRun.Knowledge = run.Knowledge
	}
	return g.client.Start(ctx, gatewayRun)
}
func (g *PiGatewayClient) Cancel(ctx context.Context, runID string) error {
	return g.client.Cancel(ctx, runID)
}
func (g *PiGatewayClient) Decide(ctx context.Context, runID, requestID string, allowed bool) error {
	return g.client.Decide(ctx, runID, requestID, allowed)
}

// ModelContextWindow returns the model's context window as reported by the
// gateway, cached for modelInfoTTL.
func (g *PiGatewayClient) ModelContextWindow(ctx context.Context, provider, model string) (int, error) {
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

func (g *PiGatewayClient) Summarize(ctx context.Context, in GatewaySummaryRequest) (string, error) {
	messages := make([]sharedllm.PiGatewayMessage, 0, len(in.Messages))
	for _, message := range in.Messages {
		messages = append(messages, sharedllm.PiGatewayMessage{Role: message.Role, Content: message.Content})
	}
	return g.client.Summarize(ctx, sharedllm.PiGatewaySummaryRequest{UserID: in.UserID, Provider: in.Provider, Model: in.Model, SystemPrompt: in.SystemPrompt,
		PreviousSummary: in.PreviousSummary, Messages: messages, MaxTokens: in.MaxTokens, Capture: &sharedllm.RequestCapture{UserID: in.UserID}})
}
