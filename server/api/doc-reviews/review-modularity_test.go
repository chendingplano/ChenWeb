package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestModularityReviewerProcessBlockPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "The 'Calibration' procedure is duplicated verbatim in §3 and §7",
				},
			},
		},
	}
	reviewer := &modularityReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_MODULARITY"),
	}

	findings := reviewer.processBlock(context.Background(), 92, 0, 1, ReviewerConfig{
		ModelName:  "modularity-model",
		PromptText: "review modularity",
		PromptRef:  "prompt-review-modularity.md",
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
	if findings[0].Aspect != "modularity" {
		t.Fatalf("aspect=%q, want modularity", findings[0].Aspect)
	}
	if findings[0].FindingType != "modularity" {
		t.Fatalf("finding_type=%q, want modularity", findings[0].FindingType)
	}
	if findings[0].Severity != "low" {
		t.Fatalf("severity=%q, want low", findings[0].Severity)
	}
	if findings[0].Location != "10-420" {
		t.Fatalf("location=%q, want 10-420", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-modularity.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-modularity.md]", fake.promptNames)
	}
}

func TestModularityReviewerMetadata(t *testing.T) {
	r := &modularityReviewer{}
	if r.Name() != "modularity" {
		t.Fatalf("Name()=%q, want modularity", r.Name())
	}
	if r.Group() != "P2" {
		t.Fatalf("Group()=%q, want P2", r.Group())
	}
	if r.Strategy() != StrategyDocument {
		t.Fatalf("Strategy()=%v, want StrategyDocument", r.Strategy())
	}
}
