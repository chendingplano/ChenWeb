package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestNavigabilityReviewerProcessBlockPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "'see Appendix C' but the document has only Appendices A and B",
				},
			},
		},
	}
	reviewer := &navigabilityReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_NAVIGABILITY"),
	}

	findings := reviewer.processBlock(context.Background(), 92, 0, 1, ReviewerConfig{
		ModelName:  "navigability-model",
		PromptText: "review navigability",
		PromptRef:  "prompt-review-navigability.md",
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
	if findings[0].Aspect != "navigability" {
		t.Fatalf("aspect=%q, want navigability", findings[0].Aspect)
	}
	if findings[0].FindingType != "navigability" {
		t.Fatalf("finding_type=%q, want navigability", findings[0].FindingType)
	}
	if findings[0].Severity != "low" {
		t.Fatalf("severity=%q, want low", findings[0].Severity)
	}
	if findings[0].Location != "10-420" {
		t.Fatalf("location=%q, want 10-420", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-navigability.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-navigability.md]", fake.promptNames)
	}
}

func TestNavigabilityReviewerMetadata(t *testing.T) {
	r := &navigabilityReviewer{}
	if r.Name() != "navigability" {
		t.Fatalf("Name()=%q, want navigability", r.Name())
	}
	if r.Group() != "P2" {
		t.Fatalf("Group()=%q, want P2", r.Group())
	}
	if r.Strategy() != StrategyDocument {
		t.Fatalf("Strategy()=%v, want StrategyDocument", r.Strategy())
	}
}
