package agentplatformhandler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/agenttrace"
	"github.com/chendingplano/shared/go/api/llm/agentrun"
)

func normalizeTraceFromEvents(ctx context.Context, runtimeKind, runID, prompt string, events []agentrun.Event) (agenttrace.Trace, bool, error) {
	lines := make([]string, 0, len(events))
	stdoutLines := make([]string, 0, len(events))
	firstAt, lastAt, eventCount := eventBounds(events)
	for _, ev := range events {
		if ev.Kind != agentrun.EventStdout {
			continue
		}
		payload := strings.TrimSpace(ev.Payload)
		if payload == "" {
			continue
		}
		stdoutLines = append(stdoutLines, payload)
		if !json.Valid([]byte(payload)) {
			continue
		}
		lines = append(lines, payload)
	}
	if len(lines) == 0 {
		if len(stdoutLines) == 0 {
			return agenttrace.Trace{}, false, nil
		}
		return agenttrace.Trace{
			AgentKind:      runtimeKind,
			RunID:          runID,
			Input:          prompt,
			Output:         strings.Join(stdoutLines, "\n"),
			TotalLatencyMS: elapsedMillis(firstAt, lastAt),
			Events:         fallbackTraceEvents(events),
			Metadata: map[string]any{
				"trace_format": "plain_text_fallback",
				"event_count":  eventCount,
			},
		}, true, nil
	}

	reg := agenttrace.NewDefaultRegistry()
	trace, err := reg.Parse(ctx, agenttrace.ParseInput{
		AgentKind: runtimeKind,
		RunID:     runID,
		Prompt:    prompt,
		RawLog:    []byte(strings.Join(lines, "\n")),
	})
	if err != nil {
		return agenttrace.Trace{}, false, err
	}
	trace = enrichTraceFromEvents(trace, firstAt, lastAt, eventCount)
	return trace, true, nil
}

func fallbackTraceEvents(events []agentrun.Event) []agenttrace.TraceEvent {
	traceEvents := make([]agenttrace.TraceEvent, 0, len(events))
	lineIndex := 0
	for _, ev := range events {
		if ev.Kind != agentrun.EventStdout {
			continue
		}
		payload := strings.TrimSpace(ev.Payload)
		if payload == "" {
			continue
		}
		traceEvents = append(traceEvents, agenttrace.TraceEvent{
			Kind:      "stdout",
			Message:   payload,
			Timestamp: ev.At,
			Fields: map[string]any{
				"line_index": lineIndex,
			},
		})
		lineIndex++
	}
	return traceEvents
}

func enrichTraceFromEvents(trace agenttrace.Trace, firstAt, lastAt time.Time, eventCount int) agenttrace.Trace {
	if trace.TotalLatencyMS == 0 {
		trace.TotalLatencyMS = elapsedMillis(firstAt, lastAt)
	}
	if trace.Metadata == nil {
		trace.Metadata = map[string]any{}
	}
	if _, ok := trace.Metadata["trace_format"]; !ok {
		trace.Metadata["trace_format"] = "jsonl"
	}
	trace.Metadata["event_count"] = eventCount
	return trace
}

func eventBounds(events []agentrun.Event) (time.Time, time.Time, int) {
	var first, last time.Time
	count := 0
	for _, ev := range events {
		if ev.At.IsZero() {
			continue
		}
		count++
		if first.IsZero() || ev.At.Before(first) {
			first = ev.At
		}
		if last.IsZero() || ev.At.After(last) {
			last = ev.At
		}
	}
	return first, last, count
}

func elapsedMillis(first, last time.Time) int64 {
	if first.IsZero() || last.IsZero() || last.Before(first) {
		return 0
	}
	return last.Sub(first).Milliseconds()
}

func promptFromTaskSpec(spec agentrun.TaskSpec) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(spec.IssueTitle)
	b.WriteString("\n\n")
	if spec.IssueDesc != "" {
		b.WriteString(spec.IssueDesc)
		b.WriteString("\n\n")
	}
	if spec.Instructions != "" {
		b.WriteString("---\n\n## Agent Instructions\n\n")
		b.WriteString(spec.Instructions)
		b.WriteString("\n")
	}
	return b.String()
}
