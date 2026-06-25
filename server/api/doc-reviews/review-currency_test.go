package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestCurrencyReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "Section 3.2 references ISO 9001:2008, which was superseded by ISO 9001:2015",
				},
			},
		},
	}
	reviewer := &currencyReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_CURRENCY"),
	}

	findings := reviewer.processWindow(context.Background(), 93, 0, ReviewerConfig{
		ModelName:  "currency-model",
		PromptText: "review currency",
		PromptRef:  "prompt-review-currency.md",
	}, currencyWindow{
		inputJSON: `{"lines":[]}`,
		startLine: 15,
		endLine:   55,
	})

	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	if findings[0].Pass != "P3" {
		t.Fatalf("pass=%q, want P3", findings[0].Pass)
	}
	if findings[0].Aspect != "currency" {
		t.Fatalf("aspect=%q, want currency", findings[0].Aspect)
	}
	if findings[0].FindingType != "outdated" {
		t.Fatalf("finding_type=%q, want outdated", findings[0].FindingType)
	}
	if findings[0].Severity != "medium" {
		t.Fatalf("severity=%q, want medium", findings[0].Severity)
	}
	if findings[0].Location != "15-55" {
		t.Fatalf("location=%q, want 15-55", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-currency.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-currency.md]", fake.promptNames)
	}
}
