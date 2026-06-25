package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestLogicalFlowReviewerProcessBlockPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "calibration step uses a value defined two sections later",
				},
			},
		},
	}
	reviewer := &logicalFlowReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_LOGICAL_FLOW"),
	}

	findings := reviewer.processBlock(context.Background(), 91, 0, 1, ReviewerConfig{
		ModelName:  "logical-flow-model",
		PromptText: "review logical flow",
		PromptRef:  "prompt-review-logical-flow.md",
	}, pageBlock{
		inputJSON: `{"lines":[]}`,
		pageStart: 1,
		pageEnd:   20,
		lineStart: 10,
		lineEnd:   420,
	})

	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	// Defaults are applied when the LLM omits fields.
	if findings[0].Pass != "P2" {
		t.Fatalf("pass=%q, want P2", findings[0].Pass)
	}
	if findings[0].Aspect != "logical_flow" {
		t.Fatalf("aspect=%q, want logical_flow", findings[0].Aspect)
	}
	if findings[0].FindingType != "logical_flow" {
		t.Fatalf("finding_type=%q, want logical_flow", findings[0].FindingType)
	}
	if findings[0].Severity != "low" {
		t.Fatalf("severity=%q, want low", findings[0].Severity)
	}
	if findings[0].Location != "10-420" {
		t.Fatalf("location=%q, want 10-420", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-logical-flow.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-logical-flow.md]", fake.promptNames)
	}
}

func TestLogicalFlowReviewerMetadata(t *testing.T) {
	r := &logicalFlowReviewer{}
	if r.Name() != "logical_flow" {
		t.Fatalf("Name()=%q, want logical_flow", r.Name())
	}
	if r.Group() != "P2" {
		t.Fatalf("Group()=%q, want P2", r.Group())
	}
	if r.Strategy() != StrategyDocument {
		t.Fatalf("Strategy()=%v, want StrategyDocument", r.Strategy())
	}
}
