package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestExamplesReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "Section 4.1 describes the authentication flow but provides no code example",
				},
			},
		},
	}
	reviewer := &examplesReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_EXAMPLES"),
	}

	findings := reviewer.processWindow(context.Background(), 101, 0, ReviewerConfig{
		ModelName:  "examples-model",
		PromptText: "review examples",
		PromptRef:  "prompt-review-examples.md",
	}, examplesWindow{
		inputJSON: `{"lines":[]}`,
		startLine: 20,
		endLine:   60,
	})

	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	if findings[0].Pass != "P3" {
		t.Fatalf("pass=%q, want P3", findings[0].Pass)
	}
	if findings[0].Aspect != "examples" {
		t.Fatalf("aspect=%q, want examples", findings[0].Aspect)
	}
	if findings[0].FindingType != "insufficient_examples" {
		t.Fatalf("finding_type=%q, want insufficient_examples", findings[0].FindingType)
	}
	if findings[0].Severity != "medium" {
		t.Fatalf("severity=%q, want medium", findings[0].Severity)
	}
	if findings[0].Location != "20-60" {
		t.Fatalf("location=%q, want 20-60", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-examples.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-examples.md]", fake.promptNames)
	}
}
