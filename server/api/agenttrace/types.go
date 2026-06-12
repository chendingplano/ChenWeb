// Package agenttrace normalizes coding-agent telemetry into a stable trace
// shape that ChenWeb can score, persist, and export independent of agent vendor.
package agenttrace

import (
	"context"
	"encoding/json"
	"os"
	"time"
)

type TokenUsage struct {
	InputTokens           int `json:"input_tokens,omitempty"`
	CachedInputTokens     int `json:"cached_input_tokens,omitempty"`
	OutputTokens          int `json:"output_tokens,omitempty"`
	ReasoningOutputTokens int `json:"reasoning_output_tokens,omitempty"`
	TotalTokens           int `json:"total_tokens,omitempty"`
}

func (u TokenUsage) withTotal() TokenUsage {
	if u.TotalTokens == 0 {
		u.TotalTokens = u.InputTokens + u.OutputTokens
	}
	return u
}

type ToolCall struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Output    string         `json:"output,omitempty"`
	Error     string         `json:"error,omitempty"`
	LatencyMS int64          `json:"latency_ms,omitempty"`
	Tokens    int            `json:"tokens,omitempty"`
}

type TraceEvent struct {
	Kind      string         `json:"kind"`
	Message   string         `json:"message,omitempty"`
	Timestamp time.Time      `json:"timestamp,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
}

type Trace struct {
	AgentKind        string         `json:"agent_kind"`
	ProviderTraceID  string         `json:"provider_trace_id,omitempty"`
	RunID            string         `json:"run_id,omitempty"`
	Input            string         `json:"input,omitempty"`
	Output           string         `json:"output,omitempty"`
	ToolCalls        []ToolCall     `json:"tool_calls,omitempty"`
	RetrievedContext []string       `json:"retrieved_context,omitempty"`
	Usage            TokenUsage     `json:"usage,omitempty"`
	TotalCostUSD     float64        `json:"total_cost_usd,omitempty"`
	TotalLatencyMS   int64          `json:"total_latency_ms,omitempty"`
	NumRetries       int            `json:"num_retries,omitempty"`
	Events           []TraceEvent   `json:"events,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

func (t Trace) ToolNames() []string {
	names := make([]string, 0, len(t.ToolCalls))
	for _, call := range t.ToolCalls {
		names = append(names, call.Name)
	}
	return names
}

func (t Trace) GetToolCalls(name string) []ToolCall {
	matches := make([]ToolCall, 0)
	for _, call := range t.ToolCalls {
		if call.Name == name {
			matches = append(matches, call)
		}
	}
	return matches
}

func (t Trace) ToJSONFile(path string) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func TraceFromJSONFile(path string) (Trace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Trace{}, err
	}
	var trace Trace
	if err := json.Unmarshal(data, &trace); err != nil {
		return Trace{}, err
	}
	return trace, nil
}

type ParseInput struct {
	AgentKind string
	RunID     string
	Prompt    string
	RawLog    []byte
	Metadata  map[string]any
}

type Plugin interface {
	Kind() string
	Parse(context.Context, ParseInput) (Trace, error)
}

type PluginFunc struct {
	KindValue string
	ParseFunc func(context.Context, ParseInput) (Trace, error)
}

func (p PluginFunc) Kind() string { return p.KindValue }

func (p PluginFunc) Parse(ctx context.Context, in ParseInput) (Trace, error) {
	return p.ParseFunc(ctx, in)
}
