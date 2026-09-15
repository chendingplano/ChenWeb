package agentservicehandler

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type GatewaySource struct {
	KnowledgeStoreID string `json:"knowledge_store_id"`
	DocumentID       string `json:"document_id"`
	ArtifactID       string `json:"artifact_id,omitempty"`
	ArtifactType     string `json:"artifact_type,omitempty"`
	SourceTitle      string `json:"source_title,omitempty"`
	SourceVersion    string `json:"source_version,omitempty"`
	Fingerprint      string `json:"source_fingerprint"`
	Page             int    `json:"page,omitempty"`
	LineStart        int    `json:"line_start,omitempty"`
	LineEnd          int    `json:"line_end,omitempty"`
	ValidationStatus string `json:"validation_status,omitempty"`
	ToolCallID       string `json:"-"`
	ToolName         string `json:"-"`
}

type AgentPublicEvent struct {
	Type             string          `json:"type"`
	Text             string          `json:"text,omitempty"`
	Status           string          `json:"status,omitempty"`
	Tool             string          `json:"tool,omitempty"`
	ToolCallID       string          `json:"toolCallId,omitempty"`
	ToolName         string          `json:"toolName,omitempty"`
	Error            bool            `json:"error,omitempty"`
	RequestID        string          `json:"requestId,omitempty"`
	Sources          []GatewaySource `json:"sources,omitempty"`
	InputTokens      int64           `json:"inputTokens,omitempty"`
	OutputTokens     int64           `json:"outputTokens,omitempty"`
	CacheReadTokens  int64           `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens int64           `json:"cacheWriteTokens,omitempty"`
	Message          string          `json:"message,omitempty"`
	Attempt          int             `json:"attempt,omitempty"`
}

func WriteAgentSSE(w io.Writer, event AgentPublicEvent) error {
	encoded, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, encoded)
	return err
}

type CompletedToolCall struct {
	GatewayID string
	ToolName  string
	Failed    bool
}
type RunEventResult struct {
	Answer       string
	Status       string
	Sources      []GatewaySource
	ToolCalls    []CompletedToolCall
	InputTokens  int64
	OutputTokens int64
}

type RunEventCollector struct {
	runID     string
	maxAnswer int
	result    RunEventResult
	completed bool
	toolCalls map[string]CompletedToolCall
}

func NewRunEventCollector(runID string, maxAnswerBytes int) *RunEventCollector {
	return &RunEventCollector{runID: runID, maxAnswer: maxAnswerBytes, toolCalls: make(map[string]CompletedToolCall), result: RunEventResult{Sources: []GatewaySource{}, ToolCalls: []CompletedToolCall{}}}
}

func (c *RunEventCollector) Accept(line []byte) (AgentPublicEvent, error) {
	if c == nil || len(line) > 128*1024 {
		return AgentPublicEvent{}, errors.New("unsafe gateway event")
	}
	var event AgentPublicEvent
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		return AgentPublicEvent{}, errors.New("invalid gateway event")
	}
	if c.completed && event.Type != "completion" {
		return AgentPublicEvent{}, errors.New("gateway event after completion")
	}
	if c.completed && event.Type == "completion" {
		return AgentPublicEvent{}, nil
	}
	switch event.Type {
	case "answer_delta":
		if len(event.Text) == 0 || len(c.result.Answer)+len(event.Text) > c.maxAnswer {
			return AgentPublicEvent{}, errors.New("agent answer limit exceeded")
		}
		c.result.Answer += event.Text
	case "activity":
		if event.Status == "retrying" && event.Attempt >= 0 && event.Attempt <= 5 && event.Tool == "" && event.ToolCallID == "" {
			return event, nil
		}
		if !stringIn(knowledgeToolNames, event.Tool) || event.ToolCallID == "" || (event.Status != "started" && event.Status != "completed") {
			return AgentPublicEvent{}, errors.New("unsafe tool activity")
		}
		if event.Status == "completed" {
			c.toolCalls[event.ToolCallID] = CompletedToolCall{GatewayID: event.ToolCallID, ToolName: event.Tool, Failed: event.Error}
		}
	case "sources":
		if !stringIn(knowledgeToolNames, event.ToolName) || event.ToolCallID == "" || len(event.Sources) > MaxToolResults || len(c.result.Sources)+len(event.Sources) > 100 {
			return AgentPublicEvent{}, errors.New("unsafe source event")
		}
		for i := range event.Sources {
			source := &event.Sources[i]
			if source.DocumentID == "" || len(source.DocumentID) > 128 || source.KnowledgeStoreID == "" || len(source.KnowledgeStoreID) > 128 || source.Fingerprint == "" || len(source.Fingerprint) > 128 || len(source.SourceTitle) > 200 || len(source.SourceVersion) > 128 || source.LineStart < 0 || source.LineEnd < source.LineStart || source.Page < 0 {
				return AgentPublicEvent{}, errors.New("incomplete source identity")
			}
			source.ToolCallID, source.ToolName = event.ToolCallID, event.ToolName
			c.result.Sources = append(c.result.Sources, *source)
		}
	case "usage":
		if event.InputTokens < 0 || event.OutputTokens < 0 {
			return AgentPublicEvent{}, errors.New("invalid usage")
		}
		c.result.InputTokens += event.InputTokens
		c.result.OutputTokens += event.OutputTokens
	case "permission_request":
		if event.RequestID == "" || !stringIn(knowledgeToolNames, event.Tool) {
			return AgentPublicEvent{}, errors.New("unsafe permission request")
		}
	case "error":
		if len(event.Message) > 300 || strings.Contains(strings.ToLower(event.Message), "api key") {
			return AgentPublicEvent{}, errors.New("unsafe gateway error")
		}
	case "completion":
		if event.Status != "completed" && event.Status != "failed" && event.Status != "cancelled" && event.Status != "timed_out" && event.Status != "limit_reached" {
			return AgentPublicEvent{}, errors.New("invalid completion status")
		}
		c.result.Status = event.Status
		c.completed = true
	default:
		return AgentPublicEvent{}, errors.New("unsafe gateway event type")
	}
	return event, nil
}

func (c *RunEventCollector) Result() RunEventResult {
	out := c.result
	out.ToolCalls = make([]CompletedToolCall, 0, len(c.toolCalls))
	for _, item := range c.toolCalls {
		out.ToolCalls = append(out.ToolCalls, item)
	}
	return out
}
