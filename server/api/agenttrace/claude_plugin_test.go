package agenttrace

import (
	"context"
	"strings"
	"testing"
)

func TestClaudeCodePluginParsesTranscriptJSONLIntoTrace(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"user","message":{"content":"List files"}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"ls"}},{"type":"text","text":"I will inspect the directory."}],"usage":{"input_tokens":80,"output_tokens":18}}}`,
		`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"README.md\nserver"}]}}`,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"The directory contains README.md and server."}],"usage":{"input_tokens":120,"output_tokens":12}}}`,
	}, "\n")

	trace, err := ClaudeCodePlugin{}.Parse(context.Background(), ParseInput{
		AgentKind: "claude_code",
		RunID:     "run-2",
		RawLog:    []byte(raw),
	})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if trace.Input != "List files" {
		t.Fatalf("input = %q, want List files", trace.Input)
	}
	if trace.Output != "The directory contains README.md and server." {
		t.Fatalf("output = %q", trace.Output)
	}
	if len(trace.ToolCalls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(trace.ToolCalls))
	}
	if trace.ToolCalls[0].Name != "Bash" || trace.ToolCalls[0].Output != "README.md\nserver" {
		t.Fatalf("unexpected tool call: %#v", trace.ToolCalls[0])
	}
	if trace.Usage.InputTokens != 200 || trace.Usage.OutputTokens != 30 || trace.Usage.TotalTokens != 230 {
		t.Fatalf("unexpected usage: %#v", trace.Usage)
	}
	if trace.Events[1].Fields["tool_use_count"] != 1 {
		t.Fatalf("expected assistant event tool count: %#v", trace.Events[1])
	}
}

func TestClaudeCodePluginParsesResultEvent(t *testing.T) {
	raw := strings.Join([]string{
		`{"type":"system","session_id":"session-1"}`,
		`{"type":"assistant","message":{"usage":{"input_tokens":80,"output_tokens":18},"content":[{"type":"tool_use","id":"toolu_1","name":"Bash","input":{"command":"go test ./..."}}]}}`,
		`{"type":"result","subtype":"success","duration_ms":2400,"total_cost_usd":0.0123,"result":"All tests passed."}`,
	}, "\n")

	trace, err := ClaudeCodePlugin{}.Parse(context.Background(), ParseInput{
		AgentKind: "claude_code",
		RunID:     "run-3",
		Prompt:    "Run tests",
		RawLog:    []byte(raw),
	})
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	if trace.ProviderTraceID != "session-1" {
		t.Fatalf("provider trace id = %q", trace.ProviderTraceID)
	}
	if trace.Output != "All tests passed." {
		t.Fatalf("output = %q", trace.Output)
	}
	if trace.TotalLatencyMS != 2400 || trace.TotalCostUSD != 0.0123 {
		t.Fatalf("unexpected run totals: latency=%d cost=%f", trace.TotalLatencyMS, trace.TotalCostUSD)
	}
	if trace.Events[2].Fields["subtype"] != "success" || trace.Events[2].Fields["duration_ms"] != int64(2400) {
		t.Fatalf("unexpected result event fields: %#v", trace.Events[2])
	}
}
