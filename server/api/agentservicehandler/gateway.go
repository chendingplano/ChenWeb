package agentservicehandler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
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
	return GatewayRunProfile{
		Slug: profile.Slug, Version: profile.Version, Provider: profile.Provider, Model: profile.Model,
		SystemPrompt: profile.SystemPrompt, ThinkingLevel: profile.ThinkingLevel,
		PermissionDefault: permission, AllowedTools: append([]string(nil), profile.AllowedTools...),
		MaxToolCalls: profile.Limits.MaxToolCalls, MaxElapsedMs: profile.Limits.MaxElapsed.Milliseconds(),
		MaxOutputTokens: profile.Limits.MaxOutputTokens, MaxEvidenceBytes: profile.Limits.MaxEvidenceBytes,
	}
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

type PiGatewayClient struct {
	baseURL *url.URL
	secret  string
	client  *http.Client
}

func NewPiGatewayClient(rawURL, secret string, client *http.Client) *PiGatewayClient {
	if rawURL == "" {
		rawURL = "http://127.0.0.1:4317"
	}
	parsed, _ := url.Parse(rawURL)
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Minute}
	}
	return &PiGatewayClient{baseURL: parsed, secret: secret, client: client}
}

func (g *PiGatewayClient) available() bool {
	return g != nil && g.baseURL != nil && (g.baseURL.Scheme == "http" || g.baseURL.Scheme == "https") && g.baseURL.Host != "" && len(g.secret) >= 16 && g.client != nil
}

func (g *PiGatewayClient) endpoint(path string) string {
	u := *g.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + path
	u.RawQuery = ""
	return u.String()
}

func (g *PiGatewayClient) Start(ctx context.Context, run GatewayRunRequest) (io.ReadCloser, error) {
	if !g.available() {
		return nil, errors.New("Pi gateway unavailable")
	}
	encoded, err := json.Marshal(run)
	if err != nil || len(encoded) > 128*1024 {
		return nil, errors.New("Pi run request too large")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint("/v1/runs"), bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+g.secret)
	req.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("Pi gateway refused run (%d)", response.StatusCode)
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "application/x-ndjson") {
		response.Body.Close()
		return nil, errors.New("Pi gateway returned unexpected stream type")
	}
	return response.Body, nil
}

func (g *PiGatewayClient) postControl(ctx context.Context, path string, payload any) error {
	if !g.available() {
		return errors.New("Pi gateway unavailable")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.endpoint(path), bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.secret)
	req.Header.Set("Content-Type", "application/json")
	response, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusAccepted {
		return fmt.Errorf("Pi control refused (%d)", response.StatusCode)
	}
	return nil
}

func (g *PiGatewayClient) Cancel(ctx context.Context, runID string) error {
	return g.postControl(ctx, "/v1/runs/"+url.PathEscape(runID)+"/cancel", map[string]any{})
}

func (g *PiGatewayClient) Decide(ctx context.Context, runID, requestID string, allowed bool) error {
	return g.postControl(ctx, "/v1/runs/"+url.PathEscape(runID)+"/permissions/"+url.PathEscape(requestID), map[string]bool{"allowed": allowed})
}
