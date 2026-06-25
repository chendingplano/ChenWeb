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
		[]ReviewTool{tc.toReviewTool()}, 42, logger,
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

func TestFinalizeFindingsLogsInfoWhenNoFindings(t *testing.T) {
	client := &fakeToolClient{
		responses: []*llmclients.Response{
			{Content: `{"findings":[]}`},
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
	if len(findings) != 0 {
		t.Fatalf("findings=%v, want empty", findings)
	}
	infoEntry := findLogEntry(logger.entries, "info", "tool-use returned no findings")
	if infoEntry == nil {
		t.Fatal("missing no-findings info log entry")
	}
	infoArgs := logArgsToMap(infoEntry.args)
	if infoArgs["record_id"] != int64(42) || infoArgs["phase"] != "finalize" || infoArgs["turn"] != -1 {
		t.Fatalf("info args=%v", infoArgs)
	}
}

func TestFinalizeFindingsReturnsErrorWhenUnparseable(t *testing.T) {
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
	if findings != nil {
		t.Fatalf("findings=%v, want nil", findings)
	}
	if !errors.Is(err, ErrToolUseFinalizeUnparseable) {
		t.Fatalf("err=%v, want ErrToolUseFinalizeUnparseable", err)
	}
	errorEntry := findLogEntry(logger.entries, "error", "tool-use finalize returned invalid findings format")
	if errorEntry == nil {
		t.Fatal("missing finalize error log entry")
	}
	errorArgs := logArgsToMap(errorEntry.args)
	if errorArgs["record_id"] != int64(42) || errorArgs["response_preview"] != "not json at all" {
		t.Fatalf("error args=%v", errorArgs)
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
