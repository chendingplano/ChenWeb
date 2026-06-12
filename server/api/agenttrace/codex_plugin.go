package agenttrace

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type CodexPlugin struct{}

func (CodexPlugin) Kind() string { return "codex" }

func (CodexPlugin) Parse(_ context.Context, in ParseInput) (Trace, error) {
	trace := Trace{
		AgentKind: "codex",
		RunID:     in.RunID,
		Input:     strings.TrimSpace(in.Prompt),
		Metadata:  cloneMetadata(in.Metadata),
	}

	scanner := bufio.NewScanner(bytes.NewReader(in.RawLog))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var evt codexEvent
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			return Trace{}, fmt.Errorf("agenttrace: parse codex jsonl line %d: %w", lineNo, err)
		}
		switch evt.Type {
		case "thread.started":
			trace.ProviderTraceID = stringFromRaw(evt.Raw, "thread_id")
		case "item.completed":
			itemType := stringFromRaw(evt.Item, "type")
			switch itemType {
			case "agent_message":
				if text := stringFromRaw(evt.Item, "text"); text != "" {
					trace.Output = text
				}
			case "command_execution", "mcp_tool_call", "web_search":
				trace.ToolCalls = append(trace.ToolCalls, toolCallFromCodexItem(evt.Item, itemType))
			}
		case "turn.completed":
			trace.Usage = tokenUsageFromRaw(evt.Raw, "usage")
		}
		trace.Events = append(trace.Events, codexTraceEvent(evt, trace.Usage))
	}
	if err := scanner.Err(); err != nil {
		return Trace{}, err
	}
	trace.Usage = trace.Usage.withTotal()
	return trace, nil
}

func codexTraceEvent(evt codexEvent, usage TokenUsage) TraceEvent {
	fields := map[string]any{}
	if evt.Type != "" {
		fields["type"] = evt.Type
	}
	if evt.Type == "thread.started" {
		if threadID := stringFromRaw(evt.Raw, "thread_id"); threadID != "" {
			fields["thread_id"] = threadID
		}
	}
	if len(evt.Item) > 0 {
		item := mapFromRaw(evt.Item)
		for _, key := range []string{"id", "type", "status", "name"} {
			if v, ok := item[key]; ok {
				fields["item_"+key] = v
			}
		}
		if command, ok := item["command"].(string); ok && command != "" {
			fields["command"] = command
		}
	}
	if evt.Type == "turn.completed" {
		fields["input_tokens"] = usage.InputTokens
		fields["cached_input_tokens"] = usage.CachedInputTokens
		fields["output_tokens"] = usage.OutputTokens
		fields["reasoning_output_tokens"] = usage.ReasoningOutputTokens
		fields["total_tokens"] = usage.TotalTokens
	}
	return TraceEvent{Kind: evt.Type, Fields: fields}
}

type codexEvent struct {
	Type string          `json:"type"`
	Item json.RawMessage `json:"item"`
	Raw  json.RawMessage
}

func (e *codexEvent) UnmarshalJSON(data []byte) error {
	type alias codexEvent
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	a.Raw = append(a.Raw[:0], data...)
	*e = codexEvent(a)
	return nil
}

func toolCallFromCodexItem(raw json.RawMessage, itemType string) ToolCall {
	call := ToolCall{
		ID:        stringFromRaw(raw, "id"),
		Name:      itemType,
		Arguments: map[string]any{},
		Output:    stringFromRaw(raw, "output"),
		Error:     stringFromRaw(raw, "error"),
	}
	if command := stringFromRaw(raw, "command"); command != "" {
		call.Arguments["command"] = command
	}
	if query := stringFromRaw(raw, "query"); query != "" {
		call.Arguments["query"] = query
	}
	if toolName := stringFromRaw(raw, "name"); toolName != "" {
		call.Name = toolName
	}
	if len(call.Arguments) == 0 {
		call.Arguments = nil
	}
	return call
}
