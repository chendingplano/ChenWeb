package agentplatformhandler

import (
	"context"
	"os"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/agenttrace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const agentTraceTracerName = "chenweb-agent-platform"

func recordAgentTraceSpan(ctx context.Context, workspaceID string, trace agenttrace.Trace) {
	attrs := []attribute.KeyValue{
		attribute.String("workspace.id", strings.TrimSpace(workspaceID)),
		attribute.String("agent.kind", strings.TrimSpace(trace.AgentKind)),
		attribute.String("agent.run_id", strings.TrimSpace(trace.RunID)),
		attribute.String("agent.provider_trace_id", strings.TrimSpace(trace.ProviderTraceID)),
		attribute.Int64("agent.tool_call_count", int64(len(trace.ToolCalls))),
		attribute.StringSlice("agent.tool_names", trace.ToolNames()),
		attribute.Int64("llm.usage.input_tokens", int64(trace.Usage.InputTokens)),
		attribute.Int64("llm.usage.cached_input_tokens", int64(trace.Usage.CachedInputTokens)),
		attribute.Int64("llm.usage.output_tokens", int64(trace.Usage.OutputTokens)),
		attribute.Int64("llm.usage.reasoning_output_tokens", int64(trace.Usage.ReasoningOutputTokens)),
		attribute.Int64("llm.usage.total_tokens", int64(trace.Usage.TotalTokens)),
		attribute.Int64("agent.output.length", int64(len(trace.Output))),
		attribute.String("agent.trace_format", traceMetadataString(trace, "trace_format")),
		attribute.Int64("agent.event_count", traceMetadataInt64(trace, "event_count")),
		attribute.Int64("agent.retrieved_context_count", int64(len(trace.RetrievedContext))),
	}
	if trace.TotalLatencyMS > 0 {
		attrs = append(attrs, attribute.Int64("agent.total_latency_ms", trace.TotalLatencyMS))
	}
	if trace.TotalCostUSD > 0 {
		attrs = append(attrs, attribute.Float64("agent.total_cost_usd", trace.TotalCostUSD))
	}
	if os.Getenv("AGENT_TRACE_OTEL_INCLUDE_CONTENT") == "true" {
		attrs = append(attrs,
			attribute.String("agent.input", trace.Input),
			attribute.String("agent.output", trace.Output),
		)
	}

	_, span := otel.Tracer(agentTraceTracerName).Start(ctx, "agent_platform.agent_trace")
	defer span.End()
	span.SetAttributes(attrs...)
	for i, ev := range trace.Events {
		eventAttrs := []attribute.KeyValue{
			attribute.Int("agent.event.index", i),
			attribute.String("agent.event.kind", ev.Kind),
		}
		for _, attr := range traceEventAttributes(ev.Fields) {
			eventAttrs = append(eventAttrs, attr)
		}
		span.AddEvent("agent.trace_event", oteltrace.WithAttributes(eventAttrs...))
	}
	for i, call := range trace.ToolCalls {
		eventAttrs := []attribute.KeyValue{
			attribute.Int("agent.tool.index", i),
			attribute.String("agent.tool.name", call.Name),
			attribute.String("agent.tool.id", call.ID),
			attribute.Bool("agent.tool.errored", strings.TrimSpace(call.Error) != ""),
		}
		if call.LatencyMS > 0 {
			eventAttrs = append(eventAttrs, attribute.Int64("agent.tool.latency_ms", call.LatencyMS))
		}
		if os.Getenv("AGENT_TRACE_OTEL_INCLUDE_CONTENT") == "true" {
			eventAttrs = append(eventAttrs,
				attribute.String("agent.tool.output", call.Output),
				attribute.String("agent.tool.error", call.Error),
			)
		}
		span.AddEvent("agent.tool_call", oteltrace.WithAttributes(eventAttrs...))
	}
	span.SetStatus(codes.Ok, "agent trace normalized")
}

func traceEventAttributes(fields map[string]any) []attribute.KeyValue {
	if len(fields) == 0 {
		return nil
	}
	attrs := make([]attribute.KeyValue, 0, len(fields))
	for key, value := range fields {
		attrKey := "agent.event." + key
		switch v := value.(type) {
		case string:
			attrs = append(attrs, attribute.String(attrKey, v))
		case int:
			attrs = append(attrs, attribute.Int(attrKey, v))
		case int64:
			attrs = append(attrs, attribute.Int64(attrKey, v))
		case float64:
			attrs = append(attrs, attribute.Float64(attrKey, v))
		case bool:
			attrs = append(attrs, attribute.Bool(attrKey, v))
		}
	}
	return attrs
}

func traceMetadataString(trace agenttrace.Trace, key string) string {
	if trace.Metadata == nil {
		return ""
	}
	if v, ok := trace.Metadata[key].(string); ok {
		return v
	}
	return ""
}

func traceMetadataInt64(trace agenttrace.Trace, key string) int64 {
	if trace.Metadata == nil {
		return 0
	}
	switch v := trace.Metadata[key].(type) {
	case int:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		return 0
	}
}
