package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestLocalizationReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "ambiguous date format 03/04/2026",
				},
			},
		},
	}
	reviewer := &localizationReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_LOCALIZATION"),
	}

	findings := reviewer.processWindow(context.Background(), 91, 0, ReviewerConfig{
		ModelName:  "localization-model",
		PromptText: "review localization",
		PromptRef:  "prompt-review-localization.md",
	}, localizationWindow{
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
	if findings[0].Aspect != "localization" {
		t.Fatalf("aspect=%q, want localization", findings[0].Aspect)
	}
	if findings[0].FindingType != "localization" {
		t.Fatalf("finding_type=%q, want localization", findings[0].FindingType)
	}
	if findings[0].Location != "10-42" {
		t.Fatalf("location=%q, want 10-42", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-localization.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-localization.md]", fake.promptNames)
	}
}
