package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestTestableClaimsReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "Section 5.2 states 'performance must be acceptable' with no measurable threshold",
				},
			},
		},
	}
	reviewer := &testableClaimsReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_TESTABLE_CLAIMS"),
	}

	findings := reviewer.processWindow(context.Background(), 103, 0, ReviewerConfig{
		ModelName:  "testable-claims-model",
		PromptText: "review testable claims",
		PromptRef:  "prompt-review-testable-claims.md",
	}, testableClaimsWindow{
		inputJSON: `{"lines":[]}`,
		startLine: 40,
		endLine:   80,
	})

	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	if findings[0].Pass != "P3" {
		t.Fatalf("pass=%q, want P3", findings[0].Pass)
	}
	if findings[0].Aspect != "testable_claims" {
		t.Fatalf("aspect=%q, want testable_claims", findings[0].Aspect)
	}
	if findings[0].FindingType != "untestable_claim" {
		t.Fatalf("finding_type=%q, want untestable_claim", findings[0].FindingType)
	}
	if findings[0].Severity != "medium" {
		t.Fatalf("severity=%q, want medium", findings[0].Severity)
	}
	if findings[0].Location != "40-80" {
		t.Fatalf("location=%q, want 40-80", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-testable-claims.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-testable-claims.md]", fake.promptNames)
	}
}
