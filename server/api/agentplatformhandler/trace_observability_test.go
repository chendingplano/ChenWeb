package agentplatformhandler

import (
	"context"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/agenttrace"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestRecordAgentTraceSpanEmitsHyperDXFriendlyAttributes(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := newTestTraceProvider(t, exporter)
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(previous)

	recordAgentTraceSpan(context.Background(), "workspace-1", agenttrace.Trace{
		AgentKind: "codex",
		RunID:     "run-1",
		ToolCalls: []agenttrace.ToolCall{
			{Name: "command_execution"},
			{Name: "web_search"},
		},
		Usage: agenttrace.TokenUsage{InputTokens: 10, OutputTokens: 5, TotalTokens: 15},
		Events: []agenttrace.TraceEvent{
			{Kind: "thread.started", Fields: map[string]any{"thread_id": "thread-1"}},
			{Kind: "turn.completed", Fields: map[string]any{"total_tokens": 15}},
		},
		Metadata: map[string]any{
			"trace_format": "jsonl",
			"event_count":  3,
		},
	})

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Name != "agent_platform.agent_trace" {
		t.Fatalf("span name = %q", spans[0].Name)
	}
	attrs := map[string]any{}
	for _, attr := range spans[0].Attributes {
		attrs[string(attr.Key)] = attr.Value.AsInterface()
	}
	if attrs["agent.kind"] != "codex" || attrs["agent.run_id"] != "run-1" || attrs["agent.tool_call_count"] != int64(2) {
		t.Fatalf("unexpected attrs: %#v", attrs)
	}
	if attrs["llm.usage.total_tokens"] != int64(15) {
		t.Fatalf("missing token attr: %#v", attrs)
	}
	if attrs["agent.trace_format"] != "jsonl" || attrs["agent.event_count"] != int64(3) {
		t.Fatalf("missing trace metadata attrs: %#v", attrs)
	}
	eventNames := make([]string, 0, len(spans[0].Events))
	for _, ev := range spans[0].Events {
		eventNames = append(eventNames, ev.Name)
	}
	if !containsString(eventNames, "agent.trace_event") || !containsString(eventNames, "agent.tool_call") {
		t.Fatalf("expected trace timeline and tool events, got %#v", eventNames)
	}
}

func newTestTraceProvider(t *testing.T, exporter *tracetest.InMemoryExporter) *sdktrace.TracerProvider {
	t.Helper()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})
	return tp
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
