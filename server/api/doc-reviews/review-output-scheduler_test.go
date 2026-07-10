package docreviews

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestRunReviewerConcurrentLimitSkipsUnclaimedUnits(t *testing.T) {
	started := make(chan int, 4)
	releases := []chan struct{}{
		make(chan struct{}),
		make(chan struct{}),
		make(chan struct{}),
		make(chan struct{}),
	}

	logger := &captureLogger{}
	var mu sync.Mutex
	var snapshots []ReviewerProgress
	resultsCh := make(chan [][]ReviewFinding, 1)
	errCh := make(chan error, 1)
	go func() {
		results, err := runReviewerConcurrent(
			context.Background(),
			2,
			4,
			ReviewerConfig{MaxFindings: 1, MaxAnalyses: 10, ReviewDepth: 2},
			"grammar_spelling",
			logger,
			42,
			func(p ReviewerProgress) {
				mu.Lock()
				snapshots = append(snapshots, p)
				mu.Unlock()
			},
			func(ctx context.Context, i int) ([]ReviewFinding, error) {
				started <- i
				<-releases[i]
				return []ReviewFinding{{Title: "finding", FindingType: "issue"}}, nil
			},
		)
		resultsCh <- results
		errCh <- err
	}()

	first := <-started
	second := <-started
	close(releases[first])
	close(releases[second])

	select {
	case extra := <-started:
		t.Fatalf("unexpected extra started unit %d", extra)
	case <-time.After(200 * time.Millisecond):
	}
	results := <-resultsCh
	err := <-errCh
	if err != nil {
		t.Fatalf("runReviewerConcurrent err = %v, want nil", err)
	}

	startedIdxs := []int{first, second}
	unstartedIdxs := make([]int, 0, 2)
	for i := range 4 {
		if i != first && i != second {
			unstartedIdxs = append(unstartedIdxs, i)
		}
	}

	if len(results) != 4 {
		t.Fatalf("results len = %d, want 4", len(results))
	}
	if len(results[first]) != 1 || len(results[second]) != 1 {
		t.Fatalf("started results lengths = (%d,%d), want (1,1)", len(results[first]), len(results[second]))
	}
	for _, idx := range unstartedIdxs {
		if len(results[idx]) != 0 {
			t.Fatalf("results[%d] len = %d, want 0 for skipped unit", idx, len(results[idx]))
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if len(snapshots) == 0 {
		t.Fatal("expected progress snapshots")
	}
	last := snapshots[len(snapshots)-1]
	if last.CompletedUnits != 4 || last.TotalUnits != 4 || last.FindingCount != 2 {
		t.Fatalf("final progress = %+v, want completed=4 total=4 findings=2", last)
	}

	entry := findLogEntry(logger.entries, "warn", outputLimitWarningMessage)
	if entry == nil {
		t.Fatal("expected output limit warning")
	}
	got := logArgsToMap(entry.args)
	want := map[string]any{
		"reviewer":      "grammar_spelling",
		"review_depth":  2,
		"max_findings":  1,
		"max_analyses":  10,
		"findings":      2,
		"analyses":      0,
		"skipped_units": 2,
		"record_id":     int64(42),
	}
	for k, v := range want {
		if !reflect.DeepEqual(got[k], v) {
			t.Fatalf("warning arg %q = %#v, want %#v (all args=%v started=%v)", k, got[k], v, got, startedIdxs)
		}
	}
	if skipped, ok := got["skipped_unit_indexes"].([]int); !ok || !reflect.DeepEqual(skipped, unstartedIdxs) {
		t.Fatalf("skipped_unit_indexes = %#v, want %v", got["skipped_unit_indexes"], unstartedIdxs)
	}
}

func TestRunReviewerConcurrentCancellationDoesNotLogLimitWarning(t *testing.T) {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)

	started := make(chan int, 4)
	releases := []chan struct{}{
		make(chan struct{}),
		make(chan struct{}),
		make(chan struct{}),
		make(chan struct{}),
	}
	logger := &captureLogger{}

	errCh := make(chan error, 1)
	go func() {
		_, err := runReviewerConcurrent(
			ctx,
			2,
			4,
			ReviewerConfig{MaxFindings: 1, MaxAnalyses: 10, ReviewDepth: 2},
			"grammar_spelling",
			logger,
			42,
			nil,
			func(ctx context.Context, i int) ([]ReviewFinding, error) {
				started <- i
				<-releases[i]
				return nil, nil
			},
		)
		errCh <- err
	}()

	first := <-started
	second := <-started
	cancel(ErrPipelineStopped)
	close(releases[first])
	close(releases[second])

	err := <-errCh
	if !errors.Is(err, ErrPipelineStopped) {
		t.Fatalf("err = %v, want ErrPipelineStopped", err)
	}
	if entry := findLogEntry(logger.entries, "warn", outputLimitWarningMessage); entry != nil {
		t.Fatalf("unexpected output limit warning: %+v", *entry)
	}
}
