package agentservicehandler

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"

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
}
type GatewayRunRequest struct {
	RunID          string                  `json:"runId"`
	ConversationID string                  `json:"conversationId"`
	UserID         string                  `json:"userId"`
	Message        string                  `json:"message"`
	Capability     string                  `json:"capability"`
	Profile        GatewayRunProfile       `json:"profile"`
	History        []GatewayHistoryMessage `json:"history"`
}
type PiGatewayClient struct{ client *sharedllm.PiGatewayClient }

func NewPiGatewayClient(rawURL, secret string, client *http.Client) *PiGatewayClient {
	return &PiGatewayClient{client: sharedllm.NewPiGatewayClient(rawURL, secret, client)}
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
	return g.client.Start(ctx, sharedllm.PiGatewayRun{RunID: run.RunID, ConversationID: run.ConversationID, UserID: run.UserID, Message: run.Message, Capability: run.Capability, Profile: run.Profile, History: run.History, Capture: &sharedllm.RequestCapture{UserID: run.UserID}})
}
func (g *PiGatewayClient) Cancel(ctx context.Context, runID string) error {
	return g.client.Cancel(ctx, runID)
}
func (g *PiGatewayClient) Decide(ctx context.Context, runID, requestID string, allowed bool) error {
	return g.client.Decide(ctx, runID, requestID, allowed)
}
