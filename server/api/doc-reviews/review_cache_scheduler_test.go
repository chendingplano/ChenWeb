package docreviews

import (
	"context"
	"reflect"
	"testing"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestOrderReviewTasksForPromptCacheGroupsSameInputTogether(t *testing.T) {
	tasks := []reviewTask{
		{aspect: "grammar_spelling", inputKey: "window-0", inputOrder: 0},
		{aspect: "grammar_spelling", inputKey: "window-1", inputOrder: 1},
		{aspect: "clarity", inputKey: "window-0", inputOrder: 0},
		{aspect: "clarity", inputKey: "window-1", inputOrder: 1},
		{aspect: "logical_flow", inputKey: "block-0", inputOrder: 2},
		{aspect: "heading_hierarchy", inputKey: "block-0", inputOrder: 2},
	}

	ordered := orderReviewTasksForPromptCache(tasks)

	var got []string
	for _, task := range ordered {
		got = append(got, task.inputKey+":"+task.aspect)
	}
	want := []string{
		"window-0:grammar_spelling",
		"window-0:clarity",
		"window-1:grammar_spelling",
		"window-1:clarity",
		"block-0:logical_flow",
		"block-0:heading_hierarchy",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order=%v, want %v", got, want)
	}
}

func TestBuildPromptCacheReviewTasksSharesInputKeysForSameWindow(t *testing.T) {
	lines := make([]Line, 0, 250)
	for i := 1; i <= 250; i++ {
		lines = append(lines, Line{
			LineNo:  i,
			PageNo:  1 + (i-1)/50,
			Content: "line",
		})
	}
	rec := DocMetadataInputRecord{ID: 77, Title: "Cache Test"}
	logger := loggerutil.CreateDefaultLogger("MID_CWB_REVIEW_CACHE_TEST")
	fake := &fakeJSONExtractor{}
	runners := []reviewRunner{
		{
			reviewer: &clarityReviewer{client: fake, logger: logger},
			cfg: ReviewerConfig{
				ModelName:  "deepseek-v4-flash",
				PromptText: "clarity prompt",
				PromptRef:  "prompt-review-clarity.md",
			},
		},
		{
			reviewer: &concisenessReviewer{client: fake, logger: logger},
			cfg: ReviewerConfig{
				ModelName:  "deepseek-v4-flash",
				PromptText: "conciseness prompt",
				PromptRef:  "prompt-review-conciseness.md",
			},
		},
	}

	tasks, unsupported := buildPromptCacheReviewTasks(77, rec, lines, runners, nil)
	if len(unsupported) != 0 {
		t.Fatalf("unsupported=%d, want 0", len(unsupported))
	}

	ordered := orderReviewTasksForPromptCache(tasks)
	got := []string{
		ordered[0].aspect,
		ordered[1].aspect,
		ordered[2].aspect,
		ordered[3].aspect,
	}
	want := []string{"clarity", "conciseness", "clarity", "conciseness"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered aspects=%v, want %v", got, want)
	}
	if ordered[0].inputKey == "" || ordered[0].inputKey != ordered[1].inputKey {
		t.Fatalf("first window keys=%q/%q, want shared non-empty key", ordered[0].inputKey, ordered[1].inputKey)
	}
	if ordered[2].inputKey == "" || ordered[2].inputKey != ordered[3].inputKey {
		t.Fatalf("second window keys=%q/%q, want shared non-empty key", ordered[2].inputKey, ordered[3].inputKey)
	}
	if ordered[0].inputKey == ordered[2].inputKey {
		t.Fatalf("window keys should differ across windows: %q", ordered[0].inputKey)
	}
}

func TestBuildPromptCacheReviewTasksSupportsAllCurrentReviewers(t *testing.T) {
	lines := make([]Line, 0, 250)
	for i := 1; i <= 250; i++ {
		lines = append(lines, Line{
			LineNo:  i,
			PageNo:  1 + (i-1)/50,
			Content: "line",
		})
	}
	rec := DocMetadataInputRecord{ID: 77, Title: "Cache Test"}
	logger := loggerutil.CreateDefaultLogger("MID_CWB_REVIEW_CACHE_ALL_TEST")
	fake := &fakeJSONExtractor{}
	runners := []reviewRunner{
		{reviewer: &grammarSpellingReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "grammar", PromptRef: "grammar.md"}},
		{reviewer: &toneVoiceReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "tone", PromptRef: "tone.md"}},
		{reviewer: &formattingConsistencyReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "format", PromptRef: "format.md"}},
		{reviewer: &readabilityReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "readability", PromptRef: "readability.md"}},
		{reviewer: &localizationReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "localization", PromptRef: "localization.md"}},
		{reviewer: &logicalFlowReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "logical", PromptRef: "logical.md"}},
		{reviewer: &headingHierarchyReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "heading", PromptRef: "heading.md"}},
		{reviewer: &navigabilityReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "nav", PromptRef: "nav.md"}},
		{reviewer: &sectionBalanceReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "balance", PromptRef: "balance.md"}},
		{reviewer: &modularityReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "modularity", PromptRef: "modularity.md"}},
		{reviewer: &completenessReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "completeness", PromptRef: "completeness.md"}},
		{reviewer: &correctnessReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "correctness", PromptRef: "correctness.md"}},
		{reviewer: &clarityReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "clarity", PromptRef: "clarity.md"}},
		{reviewer: &concisenessReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "conciseness", PromptRef: "conciseness.md"}},
		{reviewer: &relevanceReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "relevance", PromptRef: "relevance.md"}},
		{reviewer: &currencyReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "currency", PromptRef: "currency.md"}},
		{reviewer: &examplesReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "examples", PromptRef: "examples.md"}},
		{reviewer: &diagramsReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "diagrams", PromptRef: "diagrams.md"}},
		{reviewer: &testableClaimsReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "claims", PromptRef: "claims.md"}},
		{reviewer: &evidenceRationaleReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{ModelName: "deepseek-v4-flash", PromptText: "evidence", PromptRef: "evidence.md"}},
	}

	tasks, unsupported := buildPromptCacheReviewTasks(77, rec, lines, runners, nil)
	if len(unsupported) != 0 {
		var names []string
		for _, runner := range unsupported {
			names = append(names, runner.reviewer.Name())
		}
		t.Fatalf("unsupported reviewers=%v, want none", names)
	}
	if len(tasks) == 0 {
		t.Fatal("tasks=0, want scheduler tasks for reviewers")
	}
}

func TestRunReviewTasksForPromptCacheExecutesSameInputBeforeNextInput(t *testing.T) {
	var calls []string
	tasks := orderReviewTasksForPromptCache([]reviewTask{
		{aspect: "a", inputKey: "window-0", inputOrder: 0, run: func(context.Context) ([]ReviewFinding, error) {
			calls = append(calls, "window-0:a")
			return nil, nil
		}},
		{aspect: "a", inputKey: "window-1", inputOrder: 1, run: func(context.Context) ([]ReviewFinding, error) {
			calls = append(calls, "window-1:a")
			return nil, nil
		}},
		{aspect: "b", inputKey: "window-0", inputOrder: 0, run: func(context.Context) ([]ReviewFinding, error) {
			calls = append(calls, "window-0:b")
			return nil, nil
		}},
	})

	_, errs := runReviewTasksForPromptCache(context.Background(), tasks)
	if len(errs) != 0 {
		t.Fatalf("errs=%v, want none", errs)
	}

	want := []string{"window-0:a", "window-0:b", "window-1:a"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v, want %v", calls, want)
	}
}
