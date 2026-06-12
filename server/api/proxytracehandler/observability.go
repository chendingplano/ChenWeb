package proxytracehandler

import (
	"context"
	"errors"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const proxyTraceTracerName = "chenweb-agent-proxy"

func recordProxyExchangeSpan(ctx context.Context, exchange normalizedExchange) {
	_, span := otel.Tracer(proxyTraceTracerName).Start(ctx, "agent_proxy.http_exchange")
	defer span.End()

	attrs := []attribute.KeyValue{
		attribute.String("proxy.source", exchange.Source),
		attribute.String("agent.kind", exchange.AgentKind),
		attribute.String("agent.name", exchange.AgentName),
		attribute.String("agent.session", exchange.AgentSession),
		attribute.String("proxy.session_id", exchange.SessionID),
		attribute.String("llm.provider", exchange.Provider),
		attribute.String("llm.model", exchange.Model),
		attribute.String("http.method", exchange.Method),
		attribute.String("http.url", exchange.URL),
		attribute.String("http.host", exchange.Host),
		attribute.String("http.path", exchange.Path),
		attribute.Int("http.status_code", exchange.StatusCode),
		attribute.Int64("http.duration_ms", exchange.DurationMS),
		attribute.Int64("http.request.body_bytes", int64(len(exchange.RequestBody))),
		attribute.Int64("http.response.body_bytes", int64(len(exchange.ResponseBody))),
		attribute.Int64("llm.usage.input_tokens", int64(exchange.TokenUsage.InputTokens)),
		attribute.Int64("llm.usage.cached_input_tokens", int64(exchange.TokenUsage.CachedInputTokens)),
		attribute.Int64("llm.usage.output_tokens", int64(exchange.TokenUsage.OutputTokens)),
		attribute.Int64("llm.usage.reasoning_output_tokens", int64(exchange.TokenUsage.ReasoningOutputTokens)),
		attribute.Int64("llm.usage.total_tokens", int64(exchange.TokenUsage.TotalTokens)),
	}
	if os.Getenv("MITM_TRACE_OTEL_INCLUDE_CONTENT") == "true" {
		attrs = append(attrs,
			attribute.String("http.request.body", exchange.RequestBody),
			attribute.String("http.response.body", exchange.ResponseBody),
		)
	}
	span.SetAttributes(attrs...)

	span.AddEvent("http.request", oteltrace.WithAttributes(
		attribute.String("http.method", exchange.Method),
		attribute.String("http.host", exchange.Host),
		attribute.String("http.path", exchange.Path),
	))
	span.AddEvent("http.response", oteltrace.WithAttributes(
		attribute.Int("http.status_code", exchange.StatusCode),
		attribute.Int64("llm.usage.total_tokens", int64(exchange.TokenUsage.TotalTokens)),
	))

	if exchange.Error != "" {
		span.RecordError(errors.New(exchange.Error))
		span.SetStatus(codes.Error, exchange.Error)
		return
	}
	span.SetStatus(codes.Ok, "proxy exchange captured")
}
