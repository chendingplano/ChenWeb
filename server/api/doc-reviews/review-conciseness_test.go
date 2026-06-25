package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestConcisenessReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "'In order to' on line 15 can be shortened to 'to'",
				},
			},
		},
	}
	reviewer := &concisenessReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_CONCISENESS"),
	}

	findings := reviewer.processWindow(context.Background(), 91, 0, ReviewerConfig{
		ModelName:  "conciseness-model",
		PromptText: "review conciseness",
		PromptRef:  "prompt-review-conciseness.md",
	}, concisenessWindow{
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
	if findings[0].Aspect != "conciseness" {
		t.Fatalf("aspect=%q, want conciseness", findings[0].Aspect)
	}
	if findings[0].FindingType != "verbosity" {
		t.Fatalf("finding_type=%q, want verbosity", findings[0].FindingType)
	}
	if findings[0].Severity != "low" {
		t.Fatalf("severity=%q, want low", findings[0].Severity)
	}
	if findings[0].Location != "10-42" {
		t.Fatalf("location=%q, want 10-42", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-conciseness.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-conciseness.md]", fake.promptNames)
	}
}
