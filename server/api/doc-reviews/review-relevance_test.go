package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestRelevanceReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "Lines 20-25 discuss supplier qualification, which is out of scope for a sterilization validation protocol",
				},
			},
		},
	}
	reviewer := &relevanceReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_RELEVANCE"),
	}

	findings := reviewer.processWindow(context.Background(), 92, 0, ReviewerConfig{
		ModelName:  "relevance-model",
		PromptText: "review relevance",
		PromptRef:  "prompt-review-relevance.md",
	}, relevanceWindow{
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
	if findings[0].Aspect != "relevance" {
		t.Fatalf("aspect=%q, want relevance", findings[0].Aspect)
	}
	if findings[0].FindingType != "irrelevance" {
		t.Fatalf("finding_type=%q, want irrelevance", findings[0].FindingType)
	}
	if findings[0].Severity != "low" {
		t.Fatalf("severity=%q, want low", findings[0].Severity)
	}
	if findings[0].Location != "10-42" {
		t.Fatalf("location=%q, want 10-42", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-relevance.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-relevance.md]", fake.promptNames)
	}
}
