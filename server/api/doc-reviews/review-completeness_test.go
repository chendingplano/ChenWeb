package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestCompletenessReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "TODO left in sterilization parameters",
				},
			},
		},
	}
	reviewer := &completenessReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_COMPLETENESS"),
	}

	findings := reviewer.processWindow(context.Background(), 91, 0, ReviewerConfig{
		ModelName:  "completeness-model",
		PromptText: "review completeness",
		PromptRef:  "prompt-review-completeness.md",
	}, completenessWindow{
		inputJSON: `{"lines":[]}`,
		startLine: 10,
		endLine:   42,
	})

	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	// Defaults are applied when the LLM omits fields.
	if findings[0].Pass != "P3" {
		t.Fatalf("pass=%q, want P3", findings[0].Pass)
	}
	if findings[0].Aspect != "completeness" {
		t.Fatalf("aspect=%q, want completeness", findings[0].Aspect)
	}
	if findings[0].FindingType != "incompleteness" {
		t.Fatalf("finding_type=%q, want incompleteness", findings[0].FindingType)
	}
	if findings[0].Severity != "medium" {
		t.Fatalf("severity=%q, want medium", findings[0].Severity)
	}
	if findings[0].Location != "10-42" {
		t.Fatalf("location=%q, want 10-42", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-completeness.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-completeness.md]", fake.promptNames)
	}
}
