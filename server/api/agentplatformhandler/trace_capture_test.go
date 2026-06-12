package agentplatformhandler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/llm/agentrun"
)

func TestNormalizeTraceFromEventsParsesCodexStdoutJSONL(t *testing.T) {
	start := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	events := []agentrun.Event{
		{Kind: agentrun.EventStdout, Payload: `{"type":"thread.started","thread_id":"thread-1"}`, At: start},
		{Kind: agentrun.EventStdout, Payload: `{"type":"item.completed","item":{"id":"item_1","type":"agent_message","text":"done"}}`, At: start.Add(1500 * time.Millisecond)},
		{Kind: agentrun.EventStdout, Payload: `{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}`, At: start.Add(2 * time.Second)},
	}

	trace, ok, err := normalizeTraceFromEvents(context.Background(), "codex", "run-1", "prompt", events)
	if err != nil {
		t.Fatalf("normalizeTraceFromEvents returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected trace to be available")
	}
	if trace.Output != "done" || trace.Usage.TotalTokens != 15 {
		t.Fatalf("unexpected trace: %#v", trace)
	}
	if trace.TotalLatencyMS != 2000 || trace.Metadata["event_count"] != 3 {
		t.Fatalf("expected event-derived latency and count: %#v", trace)
	}
}

func TestNormalizeTraceFromEventsFallsBackToPlainTextOutput(t *testing.T) {
	start := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	events := []agentrun.Event{
		{Kind: agentrun.EventStdout, Payload: "plain human output", At: start},
		{Kind: agentrun.EventStdout, Payload: "second line", At: start.Add(time.Second)},
	}

	trace, ok, err := normalizeTraceFromEvents(context.Background(), "codex", "run-1", "prompt", events)
	if err != nil {
		t.Fatalf("normalizeTraceFromEvents returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected plain text output to produce fallback trace")
	}
	if trace.Output != "plain human output\nsecond line" || trace.Metadata["trace_format"] != "plain_text_fallback" || trace.TotalLatencyMS != 1000 {
		t.Fatalf("unexpected fallback trace: %#v", trace)
	}
	if len(trace.Events) != 2 {
		t.Fatalf("expected fallback trace timeline events, got %#v", trace.Events)
	}
	if trace.Events[0].Kind != "stdout" || trace.Events[0].Message != "plain human output" || trace.Events[0].Fields["line_index"] != 0 {
		t.Fatalf("unexpected first fallback event: %#v", trace.Events[0])
	}
}

func TestPromptFromTaskSpecMatchesIssueMarkdown(t *testing.T) {
	spec := agentrun.TaskSpec{IssueTitle: "Fix bug", IssueDesc: "Details", Instructions: "Be careful"}
	got := promptFromTaskSpec(spec)
	for _, want := range []string{"# Fix bug", "Details", "Agent Instructions", "Be careful"} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q: %s", want, got)
		}
	}
}
