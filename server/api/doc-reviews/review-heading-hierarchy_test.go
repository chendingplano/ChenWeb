package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestHeadingHierarchyReviewerProcessBlockPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "section 2.1 is followed by 2.1.1.1, skipping the H3 level",
				},
			},
		},
	}
	reviewer := &headingHierarchyReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_HEADING_HIERARCHY"),
	}

	findings := reviewer.processBlock(context.Background(), 91, 0, 1, ReviewerConfig{
		ModelName:  "heading-hierarchy-model",
		PromptText: "review heading hierarchy",
		PromptRef:  "prompt-review-heading-hierarchy.md",
	}, pageBlock{
		inputJSON: `{"lines":[]}`,
		pageStart: 1,
		pageEnd:   20,
		lineStart: 10,
		lineEnd:   420,
	})

	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	// Defaults are applied when the LLM omits fields.
	if findings[0].Pass != "P2" {
		t.Fatalf("pass=%q, want P2", findings[0].Pass)
	}
	if findings[0].Aspect != "heading_hierarchy" {
		t.Fatalf("aspect=%q, want heading_hierarchy", findings[0].Aspect)
	}
	if findings[0].FindingType != "heading_hierarchy" {
		t.Fatalf("finding_type=%q, want heading_hierarchy", findings[0].FindingType)
	}
	if findings[0].Severity != "low" {
		t.Fatalf("severity=%q, want low", findings[0].Severity)
	}
	if findings[0].Location != "10-420" {
		t.Fatalf("location=%q, want 10-420", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-heading-hierarchy.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-heading-hierarchy.md]", fake.promptNames)
	}
}

func TestHeadingHierarchyReviewerMetadata(t *testing.T) {
	r := &headingHierarchyReviewer{}
	if r.Name() != "heading_hierarchy" {
		t.Fatalf("Name()=%q, want heading_hierarchy", r.Name())
	}
	if r.Group() != "P2" {
		t.Fatalf("Group()=%q, want P2", r.Group())
	}
	if r.Strategy() != StrategyDocument {
		t.Fatalf("Strategy()=%v, want StrategyDocument", r.Strategy())
	}
}
