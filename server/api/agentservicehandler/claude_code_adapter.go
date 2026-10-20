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

// ClaudeCodeHarness is the harness name of services run on the Claude Agent
// SDK gateway (gateways/claude-code).
const ClaudeCodeHarness = "claude-code"

// claudeCodeAdapterVersion identifies the gateway protocol this adapter speaks.
const claudeCodeAdapterVersion = "1"

// ClaudeCodeHarnessAdapter runs services on the Claude Code gateway, a
// loopback Bun service that drives the Claude Agent SDK with every built-in
// tool switched off and SemOS's knowledge tools as its only tools. The
// gateway speaks the same run protocol as the Pi gateway; it does not report
// model context windows, so SemOS uses its fallback history budget.
type ClaudeCodeHarnessAdapter struct {
	baseURL string
	secret  string
	client  *http.Client
}

func NewClaudeCodeHarnessAdapter(rawURL, secret string, client *http.Client) *ClaudeCodeHarnessAdapter {
	if client == nil {
		// No overall timeout: a run streams for as long as its profile allows.
		client = &http.Client{}
	}
	return &ClaudeCodeHarnessAdapter{baseURL: strings.TrimRight(strings.TrimSpace(rawURL), "/"), secret: secret, client: client}
}

func (a *ClaudeCodeHarnessAdapter) Describe() HarnessDescriptor {
	return HarnessDescriptor{Name: ClaudeCodeHarness, AdapterVersion: claudeCodeAdapterVersion, MaxRequestBytes: 480 * 1024,
		Capabilities: []HarnessCapability{CapabilityStreaming, CapabilityCancellation, CapabilityInteractiveToolApproval,
			CapabilityUsageReporting, CapabilityHistoryReplay, CapabilityHistorySummary}}
}

// ValidateProfile rejects profiles the Agent SDK cannot run.
func (a *ClaudeCodeHarnessAdapter) ValidateProfile(profile AgentServiceProfile) error {
	if profile.Provider != "anthropic" {
		return fmt.Errorf("harness %q runs only provider \"anthropic\", not %q", ClaudeCodeHarness, profile.Provider)
	}
	return nil
}

func (a *ClaudeCodeHarnessAdapter) post(ctx context.Context, path string, body any) (*http.Response, error) {
	if a == nil || a.baseURL == "" || a.secret == "" {
		return nil, errors.New("claude code gateway is not configured")
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+a.secret)
	request.Header.Set("Content-Type", "application/json")
	return a.client.Do(request)
}

func (a *ClaudeCodeHarnessAdapter) Start(ctx context.Context, run AgentRunRequest) (io.ReadCloser, error) {
	response, err := a.post(ctx, "/v1/runs", run)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, fmt.Errorf("claude code gateway refused run: %d", response.StatusCode)
	}
	return response.Body, nil
}

func (a *ClaudeCodeHarnessAdapter) Cancel(ctx context.Context, runID string) error {
	return a.expect(ctx, "/v1/runs/"+url.PathEscape(runID)+"/cancel", map[string]string{}, http.StatusAccepted)
}

func (a *ClaudeCodeHarnessAdapter) Decide(ctx context.Context, runID, requestID string, allowed bool) error {
	return a.expect(ctx, "/v1/runs/"+url.PathEscape(runID)+"/permissions/"+url.PathEscape(requestID), map[string]bool{"allowed": allowed}, http.StatusOK)
}

func (a *ClaudeCodeHarnessAdapter) expect(ctx context.Context, path string, body any, status int) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	response, err := a.post(ctx, path, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != status {
		return fmt.Errorf("claude code gateway returned %d", response.StatusCode)
	}
	return nil
}

// ModelContextWindow is never called: the adapter does not declare
// CapabilityContextCapacityReporting.
func (a *ClaudeCodeHarnessAdapter) ModelContextWindow(context.Context, string, string) (int, error) {
	return 0, errors.New("claude code gateway does not report context windows")
}

func (a *ClaudeCodeHarnessAdapter) Summarize(ctx context.Context, in AgentSummaryRequest) (string, error) {
	messages := make([]map[string]string, 0, len(in.Messages))
	for _, message := range in.Messages {
		messages = append(messages, map[string]string{"role": message.Role, "content": message.Content})
	}
	body := map[string]any{"provider": in.Provider, "model": in.Model, "systemPrompt": in.SystemPrompt, "messages": messages, "maxTokens": in.MaxTokens}
	if in.PreviousSummary != "" {
		body["previousSummary"] = in.PreviousSummary
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	response, err := a.post(ctx, "/v1/summaries", body)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	var out struct {
		Summary string `json:"summary"`
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("claude code gateway summary failed: %d", response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&out); err != nil || strings.TrimSpace(out.Summary) == "" {
		return "", errors.New("claude code gateway returned no summary")
	}
	return out.Summary, nil
}
