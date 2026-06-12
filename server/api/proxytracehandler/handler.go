package proxytracehandler

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/chendingplano/deepdoc/server/api/agenttrace"
	"github.com/labstack/echo/v4"
)

type IngestRequest struct {
	Source          string            `json:"source"`
	AgentKind       string            `json:"agent_kind"`
	AgentName       string            `json:"agent_name"`
	AgentSession    string            `json:"agent_session"`
	SessionID       string            `json:"session_id"`
	Method          string            `json:"method"`
	URL             string            `json:"url"`
	Host            string            `json:"host"`
	Path            string            `json:"path"`
	StatusCode      int               `json:"status_code"`
	StartedAt       string            `json:"started_at"`
	DurationMS      int64             `json:"duration_ms"`
	RequestHeaders  map[string]string `json:"request_headers"`
	ResponseHeaders map[string]string `json:"response_headers"`
	RequestBody     string            `json:"request_body"`
	ResponseBody    string            `json:"response_body"`
	Error           string            `json:"error"`
}

type normalizedExchange struct {
	IngestRequest
	Provider   string
	Model      string
	TokenUsage agenttrace.TokenUsage
}

func IngestMitmExchange(c echo.Context) error {
	expected := strings.TrimSpace(os.Getenv("MITM_TRACE_INGEST_TOKEN"))
	if expected == "" {
		return c.JSON(http.StatusServiceUnavailable, map[string]any{
			"status": false,
			"error":  "MITM_TRACE_INGEST_TOKEN is not configured",
		})
	}
	if !validToken(c.Request().Header.Get(echo.HeaderAuthorization), c.Request().Header.Get("X-Trace-Token"), expected) {
		return c.JSON(http.StatusForbidden, map[string]any{
			"status": false,
			"error":  "invalid trace ingest token",
		})
	}

	var req IngestRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"status": false,
			"error":  "invalid JSON body",
		})
	}

	normalized := normalizeExchange(req)
	recordProxyExchangeSpan(c.Request().Context(), normalized)
	return c.JSON(http.StatusAccepted, map[string]any{"status": true})
}

func normalizeExchange(in IngestRequest) normalizedExchange {
	out := normalizedExchange{IngestRequest: in}
	out.Source = defaultString(strings.TrimSpace(out.Source), "mitmproxy")
	out.Method = defaultString(strings.ToUpper(strings.TrimSpace(out.Method)), http.MethodPost)
	out.Host = strings.TrimSpace(out.Host)
	out.Path = strings.TrimSpace(out.Path)
	out.URL = strings.TrimSpace(out.URL)
	out.RequestBody = truncateBody(strings.TrimSpace(out.RequestBody))
	out.ResponseBody = truncateBody(strings.TrimSpace(out.ResponseBody))
	out.Provider = detectProvider(out.Host)
	out.Model = detectModel(out.Provider, out.RequestBody, out.ResponseBody)
	out.TokenUsage = detectUsage(out.Provider, out.ResponseBody)
	return out
}

func detectProvider(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	switch {
	case strings.Contains(host, "anthropic.com"):
		return "anthropic"
	case strings.Contains(host, "openai.com"):
		return "openai"
	default:
		return ""
	}
}

func detectModel(provider, requestBody, responseBody string) string {
	for _, body := range []string{responseBody, requestBody} {
		if body == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(body), &payload); err != nil {
			continue
		}
		if model, _ := payload["model"].(string); strings.TrimSpace(model) != "" {
			return model
		}
		if provider == "openai" {
			if respModel, _ := payload["response_model"].(string); strings.TrimSpace(respModel) != "" {
				return respModel
			}
		}
	}
	return ""
}

func detectUsage(provider, responseBody string) agenttrace.TokenUsage {
	if responseBody == "" {
		return agenttrace.TokenUsage{}
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(responseBody), &payload); err != nil {
		return agenttrace.TokenUsage{}
	}
	usageRaw, ok := payload["usage"].(map[string]any)
	if !ok {
		return agenttrace.TokenUsage{}
	}
	usage := agenttrace.TokenUsage{
		InputTokens:           numberField(usageRaw, "input_tokens"),
		CachedInputTokens:     numberField(usageRaw, "cached_input_tokens"),
		OutputTokens:          numberField(usageRaw, "output_tokens"),
		ReasoningOutputTokens: numberField(usageRaw, "reasoning_output_tokens"),
		TotalTokens:           numberField(usageRaw, "total_tokens"),
	}
	if usage.TotalTokens == 0 && provider == "anthropic" {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.InputTokens + usage.OutputTokens
	}
	return usage
}

func truncateBody(body string) string {
	maxBytes := 16 * 1024
	if raw := strings.TrimSpace(os.Getenv("MITM_TRACE_MAX_BODY_BYTES")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			maxBytes = n
		}
	}
	if len(body) <= maxBytes {
		return body
	}
	return body[:maxBytes] + "\n...[truncated]"
}

func validToken(authHeader, directHeader, expected string) bool {
	if strings.TrimSpace(directHeader) == expected {
		return true
	}
	if strings.HasPrefix(authHeader, "Bearer ") && strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")) == expected {
		return true
	}
	return false
}

func numberField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
