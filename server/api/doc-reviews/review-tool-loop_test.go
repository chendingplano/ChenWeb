package docreviews

import (
	"context"
	"errors"
	"testing"

	llmclients "github.com/chendingplano/shared/go/api/llm"
	"github.com/chendingplano/shared/go/api/loggerutil"
)

// fakeToolClient implements LLMChatClient (llmclients.Client) for loop tests.
type fakeToolClient struct {
	responses []*llmclients.Response
	errs      []error
	callCount int
}

func (f *fakeToolClient) Complete(_ context.Context, _ llmclients.Request) (*llmclients.Response, error) {
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

func (f *fakeToolClient) Stream(_ context.Context, _ llmclients.Request, _ llmclients.StreamHandler) error {
	return errors.New("stream not supported in tests")
}

type countingTool struct {
	name string
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

	findings, err := runToolUseReview(
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
}

func TestRunToolUseReviewTurnBudgetExhausted(t *testing.T) {
	client := &fakeToolClient{
		responses: []*llmclients.Response{
			{Content: `{"findings":[{"title":"partial finding"}]}`, Usage: &llmclients.Usage{InputTokens: 50, OutputTokens: 30}},
		},
	}
	logger := loggerutil.CreateDefaultLogger("TEST_LOOP_BUDGET")
	// maxTurns=0 — loop should run once (clamped to 1) and then finalize.
	findings, err := runToolUseReview(
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

	_, err := runToolUseReview(
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
