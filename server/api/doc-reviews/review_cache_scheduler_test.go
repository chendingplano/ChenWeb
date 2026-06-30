package docreviews

import (
	"context"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/loggerutil"
)

func TestOrderReviewTasksForPromptCacheGroupsSameInputTogether(t *testing.T) {
	tasks := []reviewTask{
		{aspect: "grammar_spelling", inputKey: "window-0", batchID: 0, inputOrder: 0},
		{aspect: "grammar_spelling", inputKey: "window-1", batchID: 0, inputOrder: 1},
		{aspect: "clarity", inputKey: "window-0", batchID: 0, inputOrder: 0},
		{aspect: "clarity", inputKey: "window-1", batchID: 0, inputOrder: 1},
		{aspect: "logical_flow", inputKey: "block-0", batchID: 1, inputOrder: 2},
		{aspect: "heading_hierarchy", inputKey: "block-0", batchID: 1, inputOrder: 2},
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
				Input:      "per-chunk",
				ModelName:  "deepseek-v4-flash",
				PromptText: "clarity prompt",
				PromptRef:  "prompt-review-clarity.md",
			},
		},
		{
			reviewer: &concisenessReviewer{client: fake, logger: logger},
			cfg: ReviewerConfig{
				Input:      "per-chunk",
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
	chunk := "per-chunk"
	block := "per-block"
	runners := []reviewRunner{
		{reviewer: &grammarSpellingReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "grammar", PromptRef: "grammar.md"}},
		{reviewer: &toneVoiceReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "tone", PromptRef: "tone.md"}},
		{reviewer: &formattingConsistencyReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: block, ModelName: "deepseek-v4-flash", PromptText: "format", PromptRef: "format.md"}},
		{reviewer: &readabilityReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "readability", PromptRef: "readability.md"}},
		{reviewer: &localizationReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "localization", PromptRef: "localization.md"}},
		{reviewer: &logicalFlowReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: block, ModelName: "deepseek-v4-flash", PromptText: "logical", PromptRef: "logical.md"}},
		{reviewer: &headingHierarchyReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: block, ModelName: "deepseek-v4-flash", PromptText: "heading", PromptRef: "heading.md"}},
		{reviewer: &navigabilityReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: block, ModelName: "deepseek-v4-flash", PromptText: "nav", PromptRef: "nav.md"}},
		{reviewer: &sectionBalanceReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: block, ModelName: "deepseek-v4-flash", PromptText: "balance", PromptRef: "balance.md"}},
		{reviewer: &modularityReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: block, ModelName: "deepseek-v4-flash", PromptText: "modularity", PromptRef: "modularity.md"}},
		{reviewer: &completenessReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "completeness", PromptRef: "completeness.md"}},
		{reviewer: &correctnessReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "correctness", PromptRef: "correctness.md"}},
		{reviewer: &clarityReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "clarity", PromptRef: "clarity.md"}},
		{reviewer: &concisenessReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "conciseness", PromptRef: "conciseness.md"}},
		{reviewer: &relevanceReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: block, ModelName: "deepseek-v4-flash", PromptText: "relevance", PromptRef: "relevance.md"}},
		{reviewer: &currencyReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: block, ModelName: "deepseek-v4-flash", PromptText: "currency", PromptRef: "currency.md"}},
		{reviewer: &examplesReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "examples", PromptRef: "examples.md"}},
		{reviewer: &diagramsReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "diagrams", PromptRef: "diagrams.md"}},
		{reviewer: &testableClaimsReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "claims", PromptRef: "claims.md"}},
		{reviewer: &evidenceRationaleReviewer{client: fake, logger: logger}, cfg: ReviewerConfig{Input: chunk, ModelName: "deepseek-v4-flash", PromptText: "evidence", PromptRef: "evidence.md"}},
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

// TestRunReviewTasksForPromptCacheRemainingStartsDuringSeeds verifies that
// remaining reviewers start after LLM_CALL_STAGGER seconds even when seed
// tasks are still running. Seeds do NOT need to complete before remaining begin.
func TestRunReviewTasksForPromptCacheRemainingStartsDuringSeeds(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "0")

	seedsRelease := make(chan struct{})
	remainingStarted := make(chan struct{}, 2)

	makeSeed := func(inputOrder int) reviewTask {
		key := fmt.Sprintf("window-%d", inputOrder)
		return reviewTask{
			aspect:        "seed",
			inputKey:      key,
			reviewerIndex: 0,
			inputOrder:    inputOrder,
			run: func(context.Context) ([]ReviewFinding, error) {
				<-seedsRelease
				return nil, nil
			},
		}
	}
	makeRemaining := func(inputOrder int) reviewTask {
		key := fmt.Sprintf("window-%d", inputOrder)
		return reviewTask{
			aspect:        "remaining",
			inputKey:      key,
			reviewerIndex: 1,
			inputOrder:    inputOrder,
			run: func(context.Context) ([]ReviewFinding, error) {
				remainingStarted <- struct{}{}
				return nil, nil
			},
		}
	}

	tasks := orderReviewTasksForPromptCache([]reviewTask{
		makeSeed(0), makeSeed(1),
		makeRemaining(0), makeRemaining(1),
	})

	done := make(chan []error, 1)
	go func() {
		_, errs := runReviewTasksForPromptCache(context.Background(), 4, tasks)
		done <- errs
	}()

	// Both remaining tasks must start while seeds are still blocking.
	for i := range 2 {
		select {
		case <-remainingStarted:
		case <-time.After(2 * time.Second):
			t.Fatalf("remaining reviewer %d did not start while seeds were running", i+1)
		}
	}

	// Unblock seeds so everything can finish.
	close(seedsRelease)

	select {
	case errs := <-done:
		if len(errs) != 0 {
			t.Fatalf("errs=%v, want none", errs)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not finish after seeds were released")
	}
}

// TestRunReviewTasksForPromptCacheRemainingReviewersRunConcurrently verifies
// that the remaining reviewers for a single chunk (non-seeds) are launched
// concurrently and run at the same time when maxTasks allows it.
func TestRunReviewTasksForPromptCacheRemainingReviewersRunConcurrently(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "0")

	var (
		running    int32
		maxRunning int32
	)
	release := make(chan struct{})
	started := make(chan string, 2)

	makeRemaining := func(aspect string, taskOrder, reviewerIdx int) reviewTask {
		return reviewTask{
			aspect:        aspect,
			inputKey:      "window-0",
			reviewerIndex: reviewerIdx,
			inputOrder:    0,
			taskOrder:     taskOrder,
			run: func(context.Context) ([]ReviewFinding, error) {
				current := atomic.AddInt32(&running, 1)
				defer atomic.AddInt32(&running, -1)
				for {
					prev := atomic.LoadInt32(&maxRunning)
					if current <= prev || atomic.CompareAndSwapInt32(&maxRunning, prev, current) {
						break
					}
				}
				started <- aspect
				<-release
				return nil, nil
			},
		}
	}

	tasks := orderReviewTasksForPromptCache([]reviewTask{
		// reviewerIndex 0 → seed (runs synchronously, returns immediately)
		{aspect: "seed", inputKey: "window-0", reviewerIndex: 0, inputOrder: 0, taskOrder: 0,
			run: func(context.Context) ([]ReviewFinding, error) { return nil, nil }},
		makeRemaining("b", 1, 1),
		makeRemaining("c", 2, 2),
	})

	done := make(chan []error, 1)
	go func() {
		_, errs := runReviewTasksForPromptCache(context.Background(), 2, tasks)
		done <- errs
	}()

	// Both remaining reviewers should start before either is released.
	for i := range 2 {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatalf("remaining reviewer %d did not start in time", i+1)
		}
	}
	close(release)

	select {
	case errs := <-done:
		if len(errs) != 0 {
			t.Fatalf("errs=%v, want none", errs)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not finish after tasks were released")
	}
	if got := atomic.LoadInt32(&maxRunning); got != 2 {
		t.Fatalf("maxRunning=%d, want 2 (both remaining reviewers should run concurrently)", got)
	}
}

func TestMaxDocReviewerTasksUsesEnvOverride(t *testing.T) {
	t.Setenv("MAX_DOC_REVIEWER_TASKS", "3")

	if got := maxDocReviewerTasks(1); got != 3 {
		t.Fatalf("maxDocReviewerTasks=%d, want 3", got)
	}
}
