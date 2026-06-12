package agenttrace

import (
	"context"
	"strings"
	"testing"
)

func TestCodexPluginParsesJSONLEventsIntoTrace(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"thread.started","thread_id":"thread-1"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","item":{"id":"item_1","type":"command_execution","command":"bash -lc ls","status":"in_progress"}}`,
		`{"type":"item.completed","item":{"id":"item_1","type":"command_execution","command":"bash -lc ls","status":"completed","output":"README.md\nserver"}}`,
		`{"type":"item.completed","item":{"id":"item_2","type":"agent_message","text":"Repo contains README.md and server."}}`,
		`{"type":"turn.completed","usage":{"input_tokens":1000,"cached_input_tokens":700,"output_tokens":40,"reasoning_output_tokens":5}}`,
	}, "\n")

	trace, err := CodexPlugin{}.Parse(context.Background(), ParseInput{
		AgentKind: "codex",
		RunID:     "run-1",
		Prompt:    "summarize repo",
		RawLog:    []byte(raw),
	})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if trace.AgentKind != "codex" || trace.ProviderTraceID != "thread-1" {
		t.Fatalf("unexpected identifiers: %#v", trace)
	}
	if trace.Output != "Repo contains README.md and server." {
		t.Fatalf("unexpected output: %q", trace.Output)
	}
	if got := trace.Usage.TotalTokens; got != 1040 {
		t.Fatalf("total tokens = %d, want 1040", got)
	}
	if len(trace.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(trace.ToolCalls))
	}
	if trace.ToolCalls[0].Name != "command_execution" || trace.ToolCalls[0].Arguments["command"] != "bash -lc ls" {
		t.Fatalf("unexpected tool call: %#v", trace.ToolCalls[0])
	}
	if len(trace.Events) != 6 {
		t.Fatalf("events = %d, want 6", len(trace.Events))
	}
	if trace.Events[2].Fields["item_type"] != "command_execution" || trace.Events[2].Fields["item_id"] != "item_1" {
		t.Fatalf("unexpected event fields: %#v", trace.Events[2])
	}
	if trace.Events[5].Fields["total_tokens"] != 1040 {
		t.Fatalf("expected token fields on completed event: %#v", trace.Events[5])
	}
}

func TestCodexPluginRejectsMalformedJSONL(t *testing.T) {
	_, err := CodexPlugin{}.Parse(context.Background(), ParseInput{
		AgentKind: "codex",
		RawLog:    []byte(`{"type":"turn.started"}` + "\n" + `{bad json}`),
	})
	if err == nil {
		t.Fatal("expected malformed JSONL to fail")
	}
}
