package proxytracehandler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestNormalizeExchangeExtractsOpenAIUsage(t *testing.T) {
	exchange := IngestRequest{
		Source:       "mitmproxy",
		AgentKind:    "codex",
		SessionID:    "session-1",
		Method:       http.MethodPost,
		URL:          "https://api.openai.com/v1/responses",
		Host:         "api.openai.com",
		Path:         "/v1/responses",
		StatusCode:   200,
		RequestBody:  `{"model":"gpt-5","input":"say hi"}`,
		ResponseBody: `{"usage":{"input_tokens":12,"output_tokens":5,"total_tokens":17},"output":[{"type":"message","content":[{"type":"output_text","text":"hi"}]}]}`,
	}

	got := normalizeExchange(exchange)
	if got.Provider != "openai" {
		t.Fatalf("provider = %q, want openai", got.Provider)
	}
	if got.Model != "gpt-5" {
		t.Fatalf("model = %q, want gpt-5", got.Model)
	}
	if got.TokenUsage.TotalTokens != 17 || got.TokenUsage.InputTokens != 12 || got.TokenUsage.OutputTokens != 5 {
		t.Fatalf("unexpected token usage: %#v", got.TokenUsage)
	}
}

func TestIngestMitmExchangeRejectsMissingToken(t *testing.T) {
	t.Setenv("MITM_TRACE_INGEST_TOKEN", "secret")
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/internal/mitmproxy/ingest", bytes.NewBufferString(`{}`))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := IngestMitmExchange(c); err != nil {
		t.Fatalf("IngestMitmExchange returned error: %v", err)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestIngestMitmExchangeRecordsHyperDXSpan(t *testing.T) {
	t.Setenv("MITM_TRACE_INGEST_TOKEN", "secret")
	t.Setenv("MITM_TRACE_OTEL_INCLUDE_CONTENT", "true")

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	defer otel.SetTracerProvider(prev)

	e := echo.New()
	body, err := json.Marshal(IngestRequest{
		Source:       "mitmproxy",
		AgentKind:    "claude_code",
		AgentName:    "Claude Code VS Code",
		AgentSession: "tab-42",
		SessionID:    "flow-session",
		Method:       http.MethodPost,
		URL:          "https://api.anthropic.com/v1/messages",
		Host:         "api.anthropic.com",
		Path:         "/v1/messages",
		StatusCode:   200,
		RequestBody:  `{"model":"claude-sonnet-4-5","max_tokens":64,"messages":[{"role":"user","content":"hello"}]}`,
		ResponseBody: `{"id":"msg_123","model":"claude-sonnet-4-5","usage":{"input_tokens":21,"output_tokens":9}}`,
	})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/internal/mitmproxy/ingest", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set(echo.HeaderAuthorization, "Bearer secret")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := IngestMitmExchange(c); err != nil {
		t.Fatalf("IngestMitmExchange returned error: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusAccepted)
	}

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("ended spans = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.Name() != "agent_proxy.http_exchange" {
		t.Fatalf("span name = %q", span.Name())
	}

	attrs := map[string]string{}
	for _, attr := range span.Attributes() {
		attrs[string(attr.Key)] = attr.Value.AsString()
	}
	if attrs["proxy.source"] != "mitmproxy" {
		t.Fatalf("proxy.source = %q", attrs["proxy.source"])
	}
	if attrs["agent.kind"] != "claude_code" {
		t.Fatalf("agent.kind = %q", attrs["agent.kind"])
	}
	if attrs["llm.provider"] != "anthropic" {
		t.Fatalf("llm.provider = %q", attrs["llm.provider"])
	}
	if attrs["llm.model"] != "claude-sonnet-4-5" {
		t.Fatalf("llm.model = %q", attrs["llm.model"])
	}
	if attrs["http.host"] != "api.anthropic.com" {
		t.Fatalf("http.host = %q", attrs["http.host"])
	}
	if attrs["http.request.body"] == "" || attrs["http.response.body"] == "" {
		t.Fatal("expected request and response bodies on span when content export is enabled")
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	_ = os.Unsetenv("MITM_TRACE_INGEST_TOKEN")
	_ = os.Unsetenv("MITM_TRACE_OTEL_INCLUDE_CONTENT")
	os.Exit(code)
}
