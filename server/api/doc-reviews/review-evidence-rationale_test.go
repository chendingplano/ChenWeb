package docreviews

import (
	"context"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestEvidenceRationaleReviewerProcessWindowPropagatesPromptName(t *testing.T) {
	fake := &fakeJSONExtractor{
		out: map[string]any{
			"findings": []any{
				map[string]any{
					"message": "Section 3.4 selects AES-256 without citing any security requirement or threat model",
				},
			},
		},
	}
	reviewer := &evidenceRationaleReviewer{
		client: fake,
		logger: loggerutil.CreateDefaultLogger("TEST_EVIDENCE_RATIONALE"),
	}

	findings := reviewer.processWindow(context.Background(), 104, 0, ReviewerConfig{
		ModelName:  "evidence-rationale-model",
		PromptText: "review evidence rationale",
		PromptRef:  "prompt-review-evidence-rationale.md",
	}, evidenceRationaleWindow{
		inputJSON: `{"lines":[]}`,
		startLine: 50,
		endLine:   90,
	})

	if len(findings) != 1 {
		t.Fatalf("findings len=%d, want 1", len(findings))
	}
	if findings[0].Pass != "P3" {
		t.Fatalf("pass=%q, want P3", findings[0].Pass)
	}
	if findings[0].Aspect != "evidence_rationale" {
		t.Fatalf("aspect=%q, want evidence_rationale", findings[0].Aspect)
	}
	if findings[0].FindingType != "missing_evidence" {
		t.Fatalf("finding_type=%q, want missing_evidence", findings[0].FindingType)
	}
	if findings[0].Severity != "high" {
		t.Fatalf("severity=%q, want high", findings[0].Severity)
	}
	if findings[0].Location != "50-90" {
		t.Fatalf("location=%q, want 50-90", findings[0].Location)
	}
	if len(fake.promptNames) != 1 || fake.promptNames[0] != "prompt-review-evidence-rationale.md" {
		t.Fatalf("promptNames=%v, want [prompt-review-evidence-rationale.md]", fake.promptNames)
	}
}
