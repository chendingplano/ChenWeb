package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestDiagramsReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "Section 3.2 describes the authentication flow but provides no sequence diagram",
				},
			},
		},
	}
	reviewer := &diagramsReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_DIAGRAMS"),
	}

	findings := reviewer.processWindow(context.Background(), 102, 0, ReviewerConfig{
		ModelName:  "diagrams-model",
		PromptText: "review diagrams",
		PromptRef:  "prompt-review-diagrams.md",
	}, diagramsWindow{
		inputJSON: `{"lines":[]}`,
		startLine: 30,
		endLine:   70,
	})

	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	if findings[0].Pass != "P3" {
		t.Fatalf("pass=%q, want P3", findings[0].Pass)
	}
	if findings[0].Aspect != "diagrams" {
		t.Fatalf("aspect=%q, want diagrams", findings[0].Aspect)
	}
	if findings[0].FindingType != "diagram_issue" {
		t.Fatalf("finding_type=%q, want diagram_issue", findings[0].FindingType)
	}
	if findings[0].Severity != "medium" {
		t.Fatalf("severity=%q, want medium", findings[0].Severity)
	}
	if findings[0].Location != "30-70" {
		t.Fatalf("location=%q, want 30-70", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-diagrams.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-diagrams.md]", fake.promptNames)
	}
}
