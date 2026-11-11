package docreviews

import (
	"context"
	"database/sql"
	"errors"
	docprocessing "github.com/chendingplano/deepdoc/server/api/doc-processing"
	"sync/atomic"
	"testing"
	"time"
)

func TestPromptCacheFailureAbortsOnlyItsReviewer(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "0")
	failure := errors.New("grounding search failed")
	started := make(chan struct{})
	var cancelled atomic.Bool
	tasks := []reviewTask{
		{aspect: "compliance_review", reviewerIndex: 0, run: func(ctx context.Context) ([]ReviewFinding, error) {
			close(started)
			select {
			case <-ctx.Done():
				cancelled.Store(true)
				return nil, ctx.Err()
			case <-time.After(time.Second):
				return nil, errors.New("not cancelled")
			}
		}},
		{aspect: "compliance_review", reviewerIndex: 0, run: func(context.Context) ([]ReviewFinding, error) { <-started; return nil, failure }},
		{aspect: "independent", reviewerIndex: 1, run: func(context.Context) ([]ReviewFinding, error) {
			return []ReviewFinding{{Aspect: "independent", Title: "kept"}}, nil
		}},
	}
	findings, errs := runReviewTasksForPromptCache(context.Background(), 3, tasks)
	if !cancelled.Load() {
		t.Fatal("failed reviewer did not cancel its in-flight chunk")
	}
	if len(errs) != 1 || !errors.Is(errs[0], failure) {
		t.Fatalf("errors=%v, want original failure once", errs)
	}
	if len(findings) != 1 || findings[0].Aspect != "independent" {
		t.Fatalf("findings=%v, want independent reviewer only", findings)
	}
}

func TestPromptCacheDiscardsFailedReviewerOutput(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "0")
	completed := make(chan struct{})
	failure := errors.New("model failed")
	tasks := []reviewTask{
		{aspect: "broken", reviewerIndex: 0, run: func(context.Context) ([]ReviewFinding, error) {
			defer close(completed)
			return []ReviewFinding{{Aspect: "broken"}}, nil
		}},
		{aspect: "broken", reviewerIndex: 0, run: func(context.Context) ([]ReviewFinding, error) { <-completed; return nil, failure }},
	}
	findings, errs := runReviewTasksForPromptCache(context.Background(), 2, tasks)
	if len(errs) != 1 || !errors.Is(errs[0], failure) || len(findings) != 0 {
		t.Fatalf("findings=%v errors=%v", findings, errs)
	}
}

type noChunkReviewer struct{ calls atomic.Int32 }

func (*noChunkReviewer) Name() string             { return "no_chunk" }
func (*noChunkReviewer) Group() string            { return "P5" }
func (*noChunkReviewer) Strategy() ReviewStrategy { return StrategyDocument }
func (r *noChunkReviewer) ReviewDocument(context.Context, int64, ReviewerConfig) ([]ReviewFinding, error) {
	r.calls.Add(1)
	return nil, nil
}

func TestConfiguredChunkReviewerNeverFallsBack(t *testing.T) {
	r := &noChunkReviewer{}
	tasks, documentReviewers := buildPromptCacheReviewTasks(1, DocMetadataInputRecord{}, []Line{{LineNo: 1, PageNo: 1, Content: "content"}}, []reviewRunner{{reviewer: r, cfg: ReviewerConfig{Input: "per-chunk"}}}, nil)
	if len(documentReviewers) != 0 {
		t.Fatal("configured chunk reviewer was sent to document dispatch")
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks=%d, want failed dispatch task", len(tasks))
	}
	_, errs := runReviewTasksForPromptCache(context.Background(), 1, tasks)
	if len(errs) != 1 || r.calls.Load() != 0 {
		t.Fatalf("errors=%v document calls=%d", errs, r.calls.Load())
	}
}

func TestCompletedEmptySearchIsNormal(t *testing.T) {
	tasks := []reviewTask{{aspect: "compliance_review", run: func(context.Context) ([]ReviewFinding, error) { return nil, nil }}}
	findings, errs := runReviewTasksForPromptCache(context.Background(), 1, tasks)
	if len(errs) != 0 || len(findings) != 0 {
		t.Fatalf("empty completed result: findings=%v errors=%v", findings, errs)
	}
}

type terminalStatusRecorder struct{ results map[string]error }

func (*terminalStatusRecorder) UpdateAspectProgress(context.Context, int64, string, float64, int) error {
	return nil
}
func (s *terminalStatusRecorder) FinishAspect(_ context.Context, _ int64, aspect string, err error) error {
	s.results[aspect] = err
	return nil
}

func TestFinishReviewAspectsKeepsFailuresIndependent(t *testing.T) {
	store := &terminalStatusRecorder{results: map[string]error{}}
	p := &ReviewProcessor{StatusStore: store}
	failure := errors.New("model error")
	runners := []reviewRunner{{reviewer: &noChunkReviewer{}}, {reviewer: &clarityReviewer{}}}
	errs := p.finishReviewAspects(context.Background(), 1, runners, []error{&reviewerExecutionError{Aspect: "no_chunk", Err: failure}})
	if len(errs) != 0 || !errors.Is(store.results["no_chunk"], failure) || store.results["clarity"] != nil {
		t.Fatalf("terminal results=%v errors=%v", store.results, errs)
	}
	persistenceFailure := errors.New("results were not saved")
	p.finishReviewAspects(context.Background(), 1, runners, []error{persistenceFailure})
	for _, aspect := range []string{"no_chunk", "clarity"} {
		if !errors.Is(store.results[aspect], persistenceFailure) {
			t.Fatalf("%s incorrectly succeeded after persistence failure", aspect)
		}
	}
}

type reviewInputStub struct{ err error }

func (s reviewInputStub) GetInputRecord(context.Context, int64) (DocMetadataInputRecord, error) {
	return DocMetadataInputRecord{ID: 1}, s.err
}
func (reviewInputStub) UpdateInputMetadata(context.Context, int64, docprocessing.DocMetadataUpdate) error {
	return nil
}

func TestUnavailableReviewDoesNotSucceed(t *testing.T) {
	p := &ReviewProcessor{InputStore: reviewInputStub{}, Now: time.Now, Logger: &captureLogger{}, RequestedAspects: []string{"unavailable"}}
	if err := p.PostProcessIndex(context.Background(), 1); err == nil {
		t.Fatal("unavailable requested reviewer was successful")
	}
}
func TestMissingReviewedRecordDoesNotSucceed(t *testing.T) {
	p := &ReviewProcessor{InputStore: reviewInputStub{err: sql.ErrNoRows}, Now: time.Now, Logger: &captureLogger{}}
	if err := p.PostProcessIndex(context.Background(), 1); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing record error swallowed: %v", err)
	}
}

func TestPromptCacheCancellationCannotPublishSuccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	task := reviewTask{aspect: "cancelled", run: func(context.Context) ([]ReviewFinding, error) {
		cancel()
		return []ReviewFinding{{Title: "incomplete"}}, nil
	}}
	result := runPromptCacheReviewTask(ctx, task)
	if !errors.Is(result.err, context.Canceled) || len(result.findings) != 0 {
		t.Fatalf("cancelled work reported success: %+v", result)
	}
}
