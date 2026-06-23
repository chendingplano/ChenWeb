package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestGrammarSpellingReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "Typo found",
				},
			},
		},
	}
	reviewer := &grammarSpellingReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_GRAMMAR"),
	}

	findings := reviewer.processWindow(context.Background(), 77, 0, ReviewerConfig{
		ModelName:  "grammar-model",
		PromptText: "review grammar",
		PromptRef:  "prompt-review-grammar-spelling-v1.md",
	}, grammarWindow{
		inputJSON: `{"lines":[]}`,
		startLine: 1,
		endLine:   3,
	})

	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-grammar-spelling-v1.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-grammar-spelling-v1.md]", fake.promptNames)
	}
}
