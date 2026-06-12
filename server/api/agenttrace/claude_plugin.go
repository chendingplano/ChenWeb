package agenttrace

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type ClaudeCodePlugin struct{}

func (ClaudeCodePlugin) Kind() string { return "claude_code" }

func (ClaudeCodePlugin) Parse(_ context.Context, in ParseInput) (Trace, error) {
	trace := Trace{
		AgentKind: "claude_code",
		RunID:     in.RunID,
		Input:     strings.TrimSpace(in.Prompt),
		Metadata:  cloneMetadata(in.Metadata),
	}
	pending := map[string]int{}

	scanner := bufio.NewScanner(bytes.NewReader(in.RawLog))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var evt claudeEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			return Trace{}, fmt.Errorf("agenttrace: parse claude jsonl line %d: %w", lineNo, err)
		}
		if evt.SessionID != "" && trace.ProviderTraceID == "" {
			trace.ProviderTraceID = evt.SessionID
		}
		if evt.Message.Usage != nil {
			trace.Usage.InputTokens += evt.Message.Usage.InputTokens
			trace.Usage.OutputTokens += evt.Message.Usage.OutputTokens
		}
		blocks := normalizeClaudeContent(evt.Message.Content)
		switch evt.Type {
		case "system":
			if evt.SessionID != "" {
				trace.ProviderTraceID = evt.SessionID
			}
		case "user":
			for _, block := range blocks {
				if block.Type == "text" && trace.Input == "" {
					trace.Input = block.Text
				}
				if block.Type == "tool_result" {
					if idx, ok := pending[block.ToolUseID]; ok {
						trace.ToolCalls[idx].Output = block.Content
					}
				}
			}
		case "assistant":
			textParts := []string{}
			for _, block := range blocks {
				switch block.Type {
				case "text":
					if strings.TrimSpace(block.Text) != "" {
						textParts = append(textParts, strings.TrimSpace(block.Text))
					}
				case "tool_use":
					idx := len(trace.ToolCalls)
					trace.ToolCalls = append(trace.ToolCalls, ToolCall{
						ID:        block.ID,
						Name:      block.Name,
						Arguments: block.Input,
					})
					if block.ID != "" {
						pending[block.ID] = idx
					}
				}
			}
			if len(textParts) > 0 {
				trace.Output = strings.Join(textParts, "\n")
			}
		case "result":
			if strings.TrimSpace(evt.Result) != "" {
				trace.Output = strings.TrimSpace(evt.Result)
			}
			trace.TotalLatencyMS = evt.DurationMS
			trace.TotalCostUSD = evt.TotalCostUSD
		}
		trace.Events = append(trace.Events, claudeTraceEvent(evt, blocks))
	}
	if err := scanner.Err(); err != nil {
		return Trace{}, err
	}
	trace.Usage = trace.Usage.withTotal()
	return trace, nil
}

func claudeTraceEvent(evt claudeEvent, blocks []claudeBlock) TraceEvent {
	fields := map[string]any{"type": evt.Type}
	if evt.SessionID != "" {
		fields["session_id"] = evt.SessionID
	}
	if evt.Subtype != "" {
		fields["subtype"] = evt.Subtype
	}
	if evt.DurationMS > 0 {
		fields["duration_ms"] = evt.DurationMS
	}
	if evt.TotalCostUSD > 0 {
		fields["total_cost_usd"] = evt.TotalCostUSD
	}
	if evt.Message.Usage != nil {
		fields["input_tokens"] = evt.Message.Usage.InputTokens
		fields["output_tokens"] = evt.Message.Usage.OutputTokens
		fields["total_tokens"] = evt.Message.Usage.InputTokens + evt.Message.Usage.OutputTokens
	}
	toolUseCount := 0
	toolResultCount := 0
	textCount := 0
	for _, block := range blocks {
		switch block.Type {
		case "tool_use":
			toolUseCount++
		case "tool_result":
			toolResultCount++
		case "text":
			textCount++
		}
	}
	if toolUseCount > 0 {
		fields["tool_use_count"] = toolUseCount
	}
	if toolResultCount > 0 {
		fields["tool_result_count"] = toolResultCount
	}
	if textCount > 0 {
		fields["text_block_count"] = textCount
	}
	return TraceEvent{Kind: evt.Type, Fields: fields}
}

type claudeEvent struct {
	Type         string        `json:"type"`
	SessionID    string        `json:"session_id"`
	Subtype      string        `json:"subtype"`
	Result       string        `json:"result"`
	DurationMS   int64         `json:"duration_ms"`
	TotalCostUSD float64       `json:"total_cost_usd"`
	Message      claudeMessage `json:"message"`
}

type claudeMessage struct {
	Content any          `json:"content"`
	Usage   *claudeUsage `json:"usage"`
}

type claudeUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type claudeBlock struct {
	Type      string         `json:"type"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Text      string         `json:"text,omitempty"`
	Content   string         `json:"content,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
}

func normalizeClaudeContent(content any) []claudeBlock {
	data, err := json.Marshal(content)
	if err != nil || len(data) == 0 || string(data) == "null" {
		return nil
	}
	var blocks []claudeBlock
	if err := json.Unmarshal(data, &blocks); err == nil {
		return blocks
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		return []claudeBlock{{Type: "text", Text: text}}
	}
	return nil
}
