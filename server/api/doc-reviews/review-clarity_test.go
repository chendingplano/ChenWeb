package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestClarityReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "Pronoun 'it' on line 7 has no clear antecedent",
				},
			},
		},
	}
	reviewer := &clarityReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_CLARITY"),
	}

	findings := reviewer.processWindow(context.Background(), 91, 0, ReviewerConfig{
		ModelName:  "clarity-model",
		PromptText: "review clarity",
		PromptRef:  "prompt-review-clarity.md",
	}, clarityWindow{
		inputJSON: `{"lines":[]}`,
		startLine: 10,
		endLine:   42,
	})

	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	if findings[0].Pass != "P3" {
		t.Fatalf("pass=%q, want P3", findings[0].Pass)
	}
	if findings[0].Aspect != "clarity" {
		t.Fatalf("aspect=%q, want clarity", findings[0].Aspect)
	}
	if findings[0].FindingType != "ambiguity" {
		t.Fatalf("finding_type=%q, want ambiguity", findings[0].FindingType)
	}
	if findings[0].Severity != "medium" {
		t.Fatalf("severity=%q, want medium", findings[0].Severity)
	}
	if findings[0].Location != "10-42" {
		t.Fatalf("location=%q, want 10-42", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-clarity.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-clarity.md]", fake.promptNames)
	}
}
