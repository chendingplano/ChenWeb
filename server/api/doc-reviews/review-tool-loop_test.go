package docreviews

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// fakeToolClient implements LLMChatClient (llmclients.Client) for loop tests.
type fakeToolClient struct {
	responses []*llmclients.Response
	errs      []error
	requests  []llmclients.Request
	callCount int
}

type logEntry struct {
	level   string
	message string
	args    []any
}

type captureLogger struct {
	entries []logEntry
}

func (l *captureLogger) Debug(message string, args ...any) {
	l.entries = append(l.entries, logEntry{level: "debug", message: message, args: args})
}
func (l *captureLogger) Line(message string, args ...any) {
	l.entries = append(l.entries, logEntry{level: "line", message: message, args: args})
}
func (l *captureLogger) Info(message string, args ...any) {
	l.entries = append(l.entries, logEntry{level: "info", message: message, args: args})
}
func (l *captureLogger) Warn(message string, args ...any) {
	l.entries = append(l.entries, logEntry{level: "warn", message: message, args: args})
}
func (l *captureLogger) Error(message string, args ...any) {
	l.entries = append(l.entries, logEntry{level: "error", message: message, args: args})
}
func (l *captureLogger) Trace(message string) {
	l.entries = append(l.entries, logEntry{level: "trace", message: message})
}
func (l *captureLogger) Close() {}

func findLogEntry(entries []logEntry, level, message string) *logEntry {
	for i := range entries {
		if entries[i].level == level && entries[i].message == message {
			return &entries[i]
		}
	}
	return nil
}

func logArgsToMap(args []any) map[string]any {
	out := make(map[string]any, len(args)/2)
	for i := 0; i+1 < len(args); i += 2 {
		key := fmt.Sprint(args[i])
		out[key] = args[i+1]
	}
	return out
}

func (f *fakeToolClient) Complete(_ context.Context, req llmclients.Request) (*llmclients.Response, error) {
	f.requests = append(f.requests, req)
	i := f.callCount
	f.callCount++
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	if i < len(f.responses) {
		return f.responses[i], nil
	}
	return &llmclients.Response{Content: `{"findings":[]}`}, nil
}

func TestRunToolUseReviewUsesDocumentFirstPromptLayout(t *testing.T) {
	client := &fakeToolClient{
		responses: []*llmclients.Response{
			{Content: `{"findings":[]}`},
		},
	}
	logger := loggerutil.CreateDefaultLogger("TEST_LOOP_PROMPT_CACHE")

	_, _, err := runToolUseReview(
		context.Background(), client, "test-model",
		ReviewerConfig{MaxToolTurns: 1},
		"Check rationale and evidence.", "<DOCUMENT_INPUT>\nshared document\n</DOCUMENT_INPUT>\n\n<REVIEW_TASK>\nCheck rationale and evidence.\n</REVIEW_TASK>",
		[]ReviewTool{}, 42, logger,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests=%d, want 1", len(client.requests))
	}
	messages := client.requests[0].Messages
	if len(messages) < 2 {
		t.Fatalf("messages=%d, want at least 2", len(messages))
	}
	if strings.Contains(messages[0].Content, "Check rationale") {
		t.Fatalf("system prompt contains reviewer-specific task before document: %q", messages[0].Content)
	}
	user := messages[1].Content
	docIdx := strings.Index(user, "<DOCUMENT_INPUT>")
	taskIdx := strings.Index(user, "<REVIEW_TASK>")
	if docIdx < 0 || taskIdx < 0 || docIdx > taskIdx {
		t.Fatalf("user prompt is not document-first: %q", user)
	}
}

func (f *fakeToolClient) Stream(_ context.Context, _ llmclients.Request, _ llmclients.StreamHandler) error {
	return errors.New("stream not supported in tests")
}

type countingTool struct {
	name  string
	calls *int
	args  []map[string]any
}

func (t *countingTool) toReviewTool() ReviewTool {
	return ReviewTool{
		Name:        t.name,
		Description: "test tool",
		Parameters:  []byte(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`),
		Execute: func(_ context.Context, _ int64, args map[string]any) (any, error) {
			*t.calls++
			t.args = append(t.args, args)
			return map[string]any{"ok": true}, nil
		},
	}
}

func TestRunToolUseReviewToolCallThenFindings(t *testing.T) {
	tc := &countingTool{name: "search_entities", calls: new(int)}
	client := &fakeToolClient{
		responses: []*llmclients.Response{
			// Turn 0: model requests tool call.
			{
				ToolCalls: []llmclients.ToolCall{
					{ID: "tc1", Name: "search_entities", Arguments: `{"q":"sterilizer"}`},
				},
				Usage: &llmclients.Usage{InputTokens: 100, OutputTokens: 50},
			},
			// Turn 1: model returns findings.
			{Content: `{"findings":[{"title":"missing evidence for AES-256"}]}`, Usage: &llmclients.Usage{InputTokens: 200, OutputTokens: 80}},
		},
	}
	logger := loggerutil.CreateDefaultLogger("TEST_LOOP")

	findings, usage, err := runToolUseReview(
		context.Background(), client, "test-model",
		ReviewerConfig{MaxToolTurns: 5},
		"You are a doc reviewer.", "<doc_input></doc_input><task>Check rationale</task>",
		[]ReviewTool{tc.toReviewTool()}, 42, logger,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	if *tc.calls != 1 {
		t.Fatalf("tool calls=%d, want 1", *tc.calls)
	}
	if usage == nil || usage.PromptCacheHitTokens != 0 || usage.PromptCacheMissTokens != 0 {
		t.Fatalf("usage=%+v, want zero cache tokens from test responses", usage)
	}
}

func TestRunToolUseReviewAggregatesPromptCacheTokens(t *testing.T) {
	client := &fakeToolClient{
		responses: []*llmclients.Response{
			{Content: `{"findings":[]}`, Usage: &llmclients.Usage{
				InputTokens:           100,
				OutputTokens:          10,
				TotalTokens:           110,
				PromptCacheHitTokens:  80,
				PromptCacheMissTokens: 20,
			}},
		},
	}
	logger := loggerutil.CreateDefaultLogger("TEST_LOOP_USAGE")

	_, usage, err := runToolUseReview(
		context.Background(), client, "test-model",
		ReviewerConfig{MaxToolTurns: 1},
		"You are a doc reviewer.", "<doc_input></doc_input><task>Check</task>",
		[]ReviewTool{}, 42, logger,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if usage == nil {
		t.Fatal("usage=nil, want aggregate usage")
	}
	if usage.PromptCacheHitTokens != 80 || usage.PromptCacheMissTokens != 20 {
		t.Fatalf("cache hit/miss=%d/%d, want 80/20", usage.PromptCacheHitTokens, usage.PromptCacheMissTokens)
	}
}

func TestRunToolUseReviewLogsToolCallsAndResults(t *testing.T) {
	tc := &countingTool{name: "search_entities", calls: new(int)}
	client := &fakeToolClient{
		responses: []*llmclients.Response{
			{
				ToolCalls: []llmclients.ToolCall{
					{ID: "tc1", Name: "search_entities", Arguments: `{"q":"sterilizer"}`},
				},
			},
			{Content: `{"findings":[]}`},
		},
	}
	logger := &captureLogger{}

	_, _, err := runToolUseReview(
		context.Background(), client, "test-model",
		ReviewerConfig{MaxToolTurns: 5},
		"You are a doc reviewer.", "<doc_input></doc_input><task>Check rationale</task>",
		[]ReviewTool{tc.toReviewTool()}, 42, logger,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	callEntry := findLogEntry(logger.entries, "info", "tool-use call")
	if callEntry == nil {
		t.Fatal("missing tool-use call log entry")
	}
	callArgs := logArgsToMap(callEntry.args)
	if callArgs["tool_name"] != "search_entities" || callArgs["arguments"] != `{"q":"sterilizer"}` {
		t.Fatalf("call args=%v", callArgs)
	}
	resultEntry := findLogEntry(logger.entries, "info", "tool-use result")
	if resultEntry == nil {
		t.Fatal("missing tool-use result log entry")
	}
	resultArgs := logArgsToMap(resultEntry.args)
	if resultArgs["tool_name"] != "search_entities" {
		t.Fatalf("result args=%v", resultArgs)
	}
	if resultText, ok := resultArgs["result"].(string); !ok || !strings.Contains(resultText, `"ok":true`) {
		t.Fatalf("result preview=%v", resultArgs["result"])
	}
}

func TestFinalizeFindingsLogsPreviewWhenUnparseable(t *testing.T) {
	client := &fakeToolClient{
		responses: []*llmclients.Response{
			{Content: `not json at all`},
		},
	}
	logger := &captureLogger{}

	findings, _, err := finalizeFindings(
		context.Background(), client, "test-model", 42,
		[]LLMMessage{{Role: LLMRoleUser, Content: "Check"}},
		logger,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if findings != nil {
		t.Fatalf("findings=%v, want nil", findings)
	}
	warnEntry := findLogEntry(logger.entries, "warn", "tool-use finalize produced no parseable findings")
	if warnEntry == nil {
		t.Fatal("missing finalize warning log entry")
	}
	warnArgs := logArgsToMap(warnEntry.args)
	if warnArgs["record_id"] != int64(42) || warnArgs["response_preview"] != "not json at all" {
		t.Fatalf("warn args=%v", warnArgs)
	}
}

func TestRunToolUseReviewTurnBudgetExhausted(t *testing.T) {
	client := &fakeToolClient{
		responses: []*llmclients.Response{
			{Content: `{"findings":[{"title":"partial finding"}]}`, Usage: &llmclients.Usage{InputTokens: 50, OutputTokens: 30}},
		},
	}
	logger := loggerutil.CreateDefaultLogger("TEST_LOOP_BUDGET")
	// maxTurns=0 — loop should run once (clamped to 1) and then finalize.
	findings, _, err := runToolUseReview(
		context.Background(), client, "test-model",
		ReviewerConfig{MaxToolTurns: 0},
		"You are a doc reviewer.", "<doc_input></doc_input><task>Check</task>",
		[]ReviewTool{}, 42, logger,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1 (finalized)", len(findings))
	}
}

func TestRunToolUseReviewStopMidLoop(t *testing.T) {
	client := &fakeToolClient{
		responses: []*llmclients.Response{
			{Content: `{"findings":[{"title":"pre-stop"}]}`},
		},
	}
	logger := loggerutil.CreateDefaultLogger("TEST_LOOP_STOP")
	// DocProcessing.StopContext is the package-local function, not exported
	// by review_exports. Use WithCancelCause + ErrPipelineStopped directly
	// since isCtxStopped checks errors.Is(context.Cause(ctx), ErrPipelineStopped).
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(ErrPipelineStopped)

	_, _, err := runToolUseReview(
		ctx, client, "test-model",
		ReviewerConfig{MaxToolTurns: 5},
		"You are a doc reviewer.", "<doc_input/>", []ReviewTool{}, 42, logger,
	)
	if !errors.Is(err, ErrPipelineStopped) {
		t.Fatalf("expected ErrPipelineStopped, got %v", err)
	}
}

func TestExtractJSONObjectStripsFences(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{`some prologue text {"findings":[]} trailing`, `{"findings":[]}`},
		{"```json\n{\"findings\":[]}\n```", `{"findings":[]}`},
		{"```\n{\"findings\":[]}\n```", `{"findings":[]}`},
		{"", ""},
		{"no json here", ""},
		{`{"a": {"b": 1}}`, `{"a": {"b": 1}}`},
	}
	for _, tt := range tests {
		got := extractJSONObject(tt.input)
		if got != tt.want {
			t.Errorf("extractJSONObject(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMissingRequiredArgs(t *testing.T) {
	schema := []byte(`{"type":"object","properties":{"q":{"type":"string"}},"required":["q","limit"]}`)
	missing := missingRequiredArgs(schema, map[string]any{"q": "hello"})
	if len(missing) != 1 || missing[0] != "limit" {
		t.Fatalf("missing=%v, want [limit]", missing)
	}
	all := missingRequiredArgs(schema, map[string]any{"q": "hello", "limit": 5})
	if len(all) != 0 {
		t.Fatalf("missing=%v, want empty", all)
	}
}
