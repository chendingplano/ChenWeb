package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestFormattingConsistencyReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "Mixed bullet markers",
				},
			},
		},
	}
	reviewer := &formattingConsistencyReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_FORMATTING"),
	}

	findings := reviewer.processWindow(context.Background(), 88, 0, ReviewerConfig{
		ModelName:  "formatting-model",
		PromptText: "review formatting consistency",
		PromptRef:  "prompt-review-formatting-consistency.md",
	}, formattingConsistencyWindow{
		inputJSON: `{"lines":[]}`,
		startLine: 10,
		endLine:   42,
	})

	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	// Defaults are applied when the LLM omits fields.
	if findings[0].Pass != "P1" {
		t.Fatalf("pass=%q, want P1", findings[0].Pass)
	}
	if findings[0].Aspect != "formatting_consistency" {
		t.Fatalf("aspect=%q, want formatting_consistency", findings[0].Aspect)
	}
	if findings[0].FindingType != "inconsistency" {
		t.Fatalf("finding_type=%q, want inconsistency", findings[0].FindingType)
	}
	if findings[0].Location != "10-42" {
		t.Fatalf("location=%q, want 10-42", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-formatting-consistency.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-formatting-consistency.md]", fake.promptNames)
	}
}
