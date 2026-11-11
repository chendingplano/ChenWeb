package docreviews

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestToolUseReviewCarriesUserIDAcrossTurns(t *testing.T) {
	tool := &countingTool{name: "lookup", calls: new(int)}
	client := &fakeToolClient{responses: []*llmclients.Response{
		{ToolCalls: []llmclients.ToolCall{{ID: "call-1", Name: "lookup", Arguments: `{"q":"term"}`}}},
		{Content: `{"findings":[]}`},
	}}
	ctx := docprocessing.WithLLMUserID(context.Background(), "user-123")
	_, _, err := runToolUseReview(ctx, client, "test-model", ReviewerConfig{MaxToolTurns: 2},
		"prompt", "document", []ReviewTool{tool.toReviewTool()}, 42, &captureLogger{}, "review_provisions", "", "TEST")
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 2 {
		t.Fatalf("got %d calls, want 2", len(client.requests))
	}
	for i, req := range client.requests {
		if req.UserID != "user-123" {
			t.Errorf("call %d user ID = %q", i, req.UserID)
		}
	}
}

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
	// Mirror the OpenAI-compatible API constraint: every role:"tool" message must
	// be a response to a preceding assistant message carrying matching tool_calls.
	// This is what DeepSeek enforces (HTTP 400) and what the finalize-repair path
	// previously violated by emitting tool results without structured ToolCalls.
	seen := make(map[string]bool)
	for _, m := range req.Messages {
		if m.Role == LLMRoleAssistant {
			for _, tc := range m.ToolCalls {
				seen[tc.ID] = true
			}
		}
		if m.Role == LLMRoleTool && !seen[m.ToolCallID] {
			return nil, fmt.Errorf("messages with role 'tool' must be a response to a "+
				"preceding message with 'tool_calls' (tool_call_id=%q)", m.ToolCallID)
		}
	}
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
		[]ReviewTool{}, 42, logger, "review_tool_loop", "", "MID-20260706-023",
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
	if got := client.requests[0].PromptName; got != "" {
		t.Fatalf("prompt name = %q, want empty when PromptRef is unset", got)
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
		[]ReviewTool{tc.toReviewTool()}, 42, logger, "review_tool_loop_test", "", "MID-20260706-024",
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
		[]ReviewTool{}, 42, logger, "review_tool_loop_test", "", "MID-20260706-025",
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

func TestRunToolUseReviewPassesPromptRefAndPromptDirMetadata(t *testing.T) {
	t.Setenv("PROMPT_DIR", "/Users/cding/Workspace/ChenWeb/prompts")

	client := &fakeToolClient{
		responses: []*llmclients.Response{
			{Content: `{"findings":[]}`},
		},
	}
	logger := loggerutil.CreateDefaultLogger("TEST_LOOP_PROMPT_REF")

	_, _, err := runToolUseReview(
		context.Background(), client, "test-model",
		ReviewerConfig{MaxToolTurns: 1, PromptRef: "prompt-review-metrics-v3.md"},
		"You are a doc reviewer.", "<doc_input></doc_input><task>Check</task>",
		[]ReviewTool{}, 42, logger, "review_metrics", "", "MID-20260706-011",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests=%d, want 1", len(client.requests))
	}
	req := client.requests[0]
	if req.PromptName != "prompt-review-metrics-v3.md" {
		t.Fatalf("PromptName=%q, want prompt-review-metrics-v3.md", req.PromptName)
	}
	if req.Metadata["prompt_ref"] != "prompt-review-metrics-v3.md" {
		t.Fatalf("prompt_ref metadata=%v", req.Metadata["prompt_ref"])
	}
	if req.Metadata["prompt_dir_env_var"] != "PROMPT_DIR" {
		t.Fatalf("prompt_dir_env_var metadata=%v", req.Metadata["prompt_dir_env_var"])
	}
	if req.Metadata["prompt_dir"] != "/Users/cding/Workspace/ChenWeb/prompts" {
		t.Fatalf("prompt_dir metadata=%v", req.Metadata["prompt_dir"])
	}
}

func TestRunToolUseReviewToolCallPathExecutesTool(t *testing.T) {
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
		[]ReviewTool{tc.toReviewTool()}, 42, logger, "review_tool_loop_test", "", "MID-20260706-026",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *tc.calls != 1 {
		t.Fatalf("tool calls=%d, want 1", *tc.calls)
	}
	if len(tc.args) != 1 || tc.args[0]["q"] != "sterilizer" {
		t.Fatalf("tool args=%v", tc.args)
	}
	if len(client.requests) != 2 {
		t.Fatalf("requests=%d, want 2", len(client.requests))
	}
}

func TestRunToolUseReviewLogsCompletedEmptyFindings(t *testing.T) {
	client := &fakeToolClient{responses: []*llmclients.Response{{Content: `{"findings":[]}`}}}
	logger := &captureLogger{}
	findings, _, err := runToolUseReview(context.Background(), client, "test-model",
		ReviewerConfig{MaxToolTurns: 2}, "prompt", "document", nil, 42, logger, "test", "", "TEST")
	if err != nil || len(findings) != 0 {
		t.Fatalf("findings=%v err=%v", findings, err)
	}
	entry := findLogEntry(logger.entries, "info", "tool-use returned no findings")
	if entry == nil {
		t.Fatal("missing completed-empty result log")
	}
	if got := logArgsToMap(entry.args)["phase"]; got != "response" {
		t.Fatalf("phase=%v", got)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests=%d, want 1", len(client.requests))
	}
}

func TestRunToolUseReviewAbortsMalformedOutputWithoutRepairCall(t *testing.T) {
	for _, content := range []string{"not json at all", `{"findings": [}`, `{"findings":null}`, `{"findings":"wrong type"}`, `{"findings":[null]}`, `{"findings":[{"description":"missing title"}]}`, "<｜｜DSML｜｜tool_calls><｜｜DSML｜｜invoke name=\"lookup\"></｜｜DSML｜｜invoke></｜｜DSML｜｜tool_calls>"} {
		t.Run(content, func(t *testing.T) {
			client := &fakeToolClient{responses: []*llmclients.Response{{Content: content}, {Content: `{"findings":[]}`}}}
			logger := &captureLogger{}
			findings, _, err := runToolUseReview(context.Background(), client, "test-model",
				ReviewerConfig{MaxToolTurns: 3}, "prompt", "document", nil, 42, logger, "test", "", "TEST")
			if err == nil || findings != nil {
				t.Fatalf("findings=%v err=%v, want failed unit", findings, err)
			}
			if len(client.requests) != 1 {
				t.Fatalf("requests=%d, want no repair/retry", len(client.requests))
			}
			if findLogEntry(logger.entries, "error", "tool-use response returned invalid findings format") == nil {
				t.Fatal("missing error log")
			}
		})
	}
}

func TestRunToolUseReviewAbortsToolErrorsWithoutRetry(t *testing.T) {
	boom := errors.New("tool database failed")
	cases := []struct {
		name, args, schema string
		execute            func(context.Context, int64, map[string]any) (any, error)
		wantCause          error
	}{
		{name: "unknown", args: `{}`},
		{name: "lookup", args: `{`, schema: `{"type":"object"}`},
		{name: "lookup", args: `{}`, schema: `{"type":"object","required":["q"]}`},
		{name: "lookup", args: `{"q":7}`, schema: `{"type":"object","properties":{"q":{"type":"string"}}}`},
		{name: "lookup", args: `{}`, schema: `{`},
		{name: "lookup", args: `{}`, schema: `{"type":123}`},
		{name: "lookup", args: `{}`, execute: func(context.Context, int64, map[string]any) (any, error) { return nil, boom }, wantCause: boom},
		{name: "lookup", args: `{}`, execute: func(context.Context, int64, map[string]any) (any, error) { return make(chan int), nil }},
	}
	for i, tc := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			var executions int
			tool := ReviewTool{Name: "lookup", Parameters: []byte(tc.schema), Execute: func(ctx context.Context, recordID int64, args map[string]any) (any, error) {
				executions++
				if tc.execute != nil {
					return tc.execute(ctx, recordID, args)
				}
				return nil, nil
			}}
			client := &fakeToolClient{responses: []*llmclients.Response{
				{ToolCalls: []llmclients.ToolCall{{ID: "call-1", Name: tc.name, Arguments: tc.args}, {ID: "call-2", Name: "lookup", Arguments: `{}`}}},
				{Content: `{"findings":[]}`},
			}}
			logger := &captureLogger{}
			findings, _, err := runToolUseReview(context.Background(), client, "test-model", ReviewerConfig{MaxToolTurns: 3}, "prompt", "document", []ReviewTool{tool}, 42, logger, "test", "", "TEST")
			if err == nil || findings != nil {
				t.Fatalf("findings=%v err=%v", findings, err)
			}
			if tc.wantCause != nil && !errors.Is(err, tc.wantCause) {
				t.Fatalf("err=%v does not wrap cause", err)
			}
			if len(client.requests) != 1 {
				t.Fatalf("requests=%d, want 1", len(client.requests))
			}
			wantExecutions := 0
			if tc.execute != nil {
				wantExecutions = 1
			}
			if executions != wantExecutions {
				t.Fatalf("executions=%d, want %d", executions, wantExecutions)
			}
			if findLogEntry(logger.entries, "error", "tool-use tool call failed") == nil {
				t.Fatal("missing tool failure log")
			}
		})
	}
}

func TestRunToolUseReviewAbortsLLMFailure(t *testing.T) {
	boom := errors.New("provider failed")
	for _, tc := range []struct {
		name   string
		client *fakeToolClient
		cause  error
	}{
		{name: "provider", client: &fakeToolClient{errs: []error{boom}}, cause: boom},
		{name: "missing response", client: &fakeToolClient{responses: []*llmclients.Response{nil}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := &captureLogger{}
			findings, _, err := runToolUseReview(context.Background(), tc.client, "test-model", ReviewerConfig{MaxToolTurns: 3}, "prompt", "document", nil, 42, logger, "test", "", "TEST")
			if err == nil || findings != nil {
				t.Fatalf("findings=%v err=%v", findings, err)
			}
			if tc.cause != nil && !errors.Is(err, tc.cause) {
				t.Fatalf("err=%v", err)
			}
			if len(tc.client.requests) != 1 {
				t.Fatalf("requests=%d", len(tc.client.requests))
			}
			if findLogEntry(logger.entries, "error", "tool-use LLM call failed") == nil {
				t.Fatal("missing LLM error log")
			}
		})
	}
}

func TestRunToolUseReviewAbortsInvalidCallMetadata(t *testing.T) {
	client := &fakeToolClient{}
	logger := &captureLogger{}
	_, _, err := runToolUseReview(context.Background(), client, "test-model", ReviewerConfig{MaxToolTurns: 3}, "prompt", "document", nil, 42, logger, "test", "{", "TEST")
	if err == nil {
		t.Fatal("invalid call metadata must abort")
	}
	if len(client.requests) != 0 {
		t.Fatalf("requests=%d, want none", len(client.requests))
	}
	if findLogEntry(logger.entries, "error", "tool-use call metadata is invalid") == nil {
		t.Fatal("missing metadata error log")
	}
}

func TestRunToolUseReviewAbortsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &fakeToolClient{}
	logger := &captureLogger{}
	_, _, err := runToolUseReview(ctx, client, "test-model", ReviewerConfig{MaxToolTurns: 3}, "prompt", "document", nil, 42, logger, "test", "", "TEST")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
	if len(client.requests) != 0 {
		t.Fatalf("requests=%d, want none", len(client.requests))
	}
}

func TestRunToolUseReviewBudgetExhaustionDoesNotFinalize(t *testing.T) {
	for _, cfg := range []ReviewerConfig{{MaxToolTurns: 1}, {MaxToolTurns: 3, MaxToolTokens: 1}} {
		t.Run(fmt.Sprint(cfg), func(t *testing.T) {
			tool := &countingTool{name: "lookup", calls: new(int)}
			client := &fakeToolClient{responses: []*llmclients.Response{
				{ToolCalls: []llmclients.ToolCall{{ID: "call-1", Name: "lookup", Arguments: `{"q":"term"}`}}, Usage: &llmclients.Usage{TotalTokens: 10}},
				{Content: `{"findings":[]}`},
			}}
			logger := &captureLogger{}
			findings, usage, err := runToolUseReview(context.Background(), client, "test-model", cfg, "prompt", "document", []ReviewTool{tool.toReviewTool()}, 42, logger, "test", "", "TEST")
			if err == nil || findings != nil {
				t.Fatalf("findings=%v err=%v", findings, err)
			}
			if len(client.requests) != 1 {
				t.Fatalf("requests=%d, want 1", len(client.requests))
			}
			if *tool.calls != 0 {
				t.Fatalf("tools=%d, want no unnecessary work after exhaustion", *tool.calls)
			}
			if usage == nil || usage.TotalTokens != 10 {
				t.Fatalf("usage=%v", usage)
			}
			if findLogEntry(logger.entries, "error", "tool-use review budget exhausted") == nil {
				t.Fatal("missing budget failure log")
			}
		})
	}
}

func TestRunToolUseReviewRepairsJSONProgrammatically(t *testing.T) {
	client := &fakeToolClient{responses: []*llmclients.Response{{Content: `{"findings":[{"title":"bad","description":"附录A指出"从设备名称字面理解"会误导"}]}`}}}
	findings, _, err := runToolUseReview(context.Background(), client, "test-model", ReviewerConfig{MaxToolTurns: 2}, "prompt", "document", nil, 42, &captureLogger{}, "test", "", "TEST")
	if err != nil || len(findings) != 1 {
		t.Fatalf("findings=%v err=%v", findings, err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("requests=%d", len(client.requests))
	}
	if !strings.Contains(findings[0].Description, `"从设备名称字面理解"`) {
		t.Fatalf("description=%q", findings[0].Description)
	}
}

// TestParseFindingsContentRepairsUnescapedInnerQuotes verifies that the
// programmatic JSON repair (unescaped ASCII double quotes inside string values)
// succeeds without a costly LLM repair round-trip.  This reproduces the
// production error "json_unmarshal_failed: invalid character 'é' after object
// key:value pair" where the model inserts literal ASCII quotes when quoting
// Chinese terms within JSON string fields.
func TestParseFindingsContentRepairsUnescapedInnerQuotes(t *testing.T) {
	// This is a simplified version of the production payload from the log —
	// ASCII double quotes inside the Chinese title/description/suggestion
	// fields are unescaped, which makes the raw JSON invalid.
	input := "```json\n" +
		"{\n" +
		"  \"findings\": [\n" +
		"    {\n" +
		"      \"severity\": \"high\",\n" +
		"      \"finding_type\": \"name_collision\",\n" +
		"      \"title\": \"同一中文名称\"霉菌生长试验箱\"指向两个不同分类的设备\",\n" +
		"      \"description\": \"编码04-11-70定义为\"霉菌生长试验箱\"\",\n" +
		"      \"evidence\": \"表B.2中：04-11-70 \"霉菌生长试验箱\"\",\n" +
		"      \"location\": \"1094\",\n" +
		"      \"suggestion\": \"建议重命名为\"霉菌培养箱\"\"\n" +
		"    }\n" +
		"  ]\n" +
		"}\n" +
		"```\n"

	findings, ok := parseFindingsContent(input)
	if !ok {
		t.Fatal("parseFindingsContent failed; repair should fix unescaped inner quotes")
	}
	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	f := findings[0]
	if f.Severity != "high" {
		t.Fatalf("severity=%q, want high", f.Severity)
	}
	if f.FindingType != "name_collision" {
		t.Fatalf("finding_type=%q, want name_collision", f.FindingType)
	}
	if f.Location != "1094" {
		t.Fatalf("location=%q, want 1094", f.Location)
	}
	// Verify the escaped quotes were repaired and the Chinese text is intact.
	if !strings.Contains(f.Title, "霉菌生长试验箱") {
		t.Fatalf("title missing expected Chinese text: %q", f.Title)
	}
}

func TestRunToolUseReviewTurnBudgetExhausted(t *testing.T) {
	client := &fakeToolClient{
		responses: []*llmclients.Response{
			{Content: `{"findings":[{"title":"partial finding"}]}`, Usage: &llmclients.Usage{InputTokens: 50, OutputTokens: 30}},
		},
	}
	logger := loggerutil.CreateDefaultLogger("TEST_LOOP_BUDGET")
	// maxTurns=0 is clamped to one call; a complete result needs no additional work.
	findings, _, err := runToolUseReview(
		context.Background(), client, "test-model",
		ReviewerConfig{MaxToolTurns: 0},
		"You are a doc reviewer.", "<doc_input></doc_input><task>Check</task>",
		[]ReviewTool{}, 42, logger, "review_tool_loop_test", "", "MID-20260706-027",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1 (completed in the initial call)", len(findings))
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
		"You are a doc reviewer.", "<doc_input/>", []ReviewTool{}, 42, logger, "review_tool_loop_test", "", "MID-20260706-028",
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

func TestParseFindingsContentWithTrailingClosingFence(t *testing.T) {
	input := "{\n" +
		"  \"findings\": [\n" +
		"    {\n" +
		"      \"severity\": \"medium\",\n" +
		"      \"finding_type\": \"outdated_regulatory_citation\",\n" +
		"      \"title\": \"引用已废止的GB/T 1.1—2009标准起草规则\",\n" +
		"      \"description\": \"本标准于2021-04-30发布，但第37行声明按照GB/T1.1—2009的规则起草。GB/T 1.1—2009已被GB/T 1.1—2020替代，新发布的国家标准应采用现行有效的起草规则，避免合规风险。\",\n" +
		"      \"evidence\": \"第37行：“本标准按照GB/T1.1—2009给出的规则起草。”\",\n" +
		"      \"location\": \"37\",\n" +
		"      \"suggestion\": \"将起草规则更新为GB/T 1.1—2020，并根据新规则调整标准的结构 与编写格式。\",\n" +
		"      \"confidence\": 0.85\n" +
		"    }\n" +
		"  ]\n" +
		"}\n" +
		"```"

	findings, ok := parseFindingsContent(input)
	if !ok {
		t.Fatal("parseFindingsContent returned ok=false, want true")
	}
	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	if findings[0].FindingType != "outdated_regulatory_citation" {
		t.Fatalf("finding_type=%q", findings[0].FindingType)
	}
}

func TestParseFindingsContentDetailedReportsUnbalancedJSONObject(t *testing.T) {
	input := "```json\n{\n  \"findings\": [\n    {\n      \"title\": \"cut off\"\n    }\n  ]\n"

	findings, ok, reason := parseFindingsContentDetailed(input)
	if ok {
		t.Fatalf("ok=true findings=%v, want false", findings)
	}
	if reason != "unbalanced_json_object" {
		t.Fatalf("reason=%q, want unbalanced_json_object", reason)
	}
}

func TestParseFindingsContentDetailedReportsTextToolCalls(t *testing.T) {
	input := "<｜｜DSML｜｜tool_calls>\n<｜｜DSML｜｜invoke name=\"search_provisions\"></｜｜DSML｜｜invoke>\n</｜｜DSML｜｜tool_calls>"

	findings, ok, reason := parseFindingsContentDetailed(input)
	if ok {
		t.Fatalf("ok=true findings=%v, want false", findings)
	}
	if reason != "tool_calls_in_final_text" {
		t.Fatalf("reason=%q, want tool_calls_in_final_text", reason)
	}
}
