package docprocessing

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// scriptedWindow is a peak window whose evaluations return script in order,
// then repeat its last entry. Poll is 1 ms, so every check after a wait
// re-evaluates.
type scriptedWindow struct {
	mu     sync.Mutex
	script []bool
	err    error
	calls  int
}

func (s *scriptedWindow) window() *PeakWindow {
	return &PeakWindow{Name: "test window", Poll: time.Millisecond, activeAt: s.activeAt}
}

func (s *scriptedWindow) activeAt(context.Context, time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return false, s.err
	}
	i := min(s.calls-1, len(s.script)-1)
	return s.script[i], nil
}

// set replaces the script with a single answer.
func (s *scriptedWindow) set(active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.script, s.calls = []bool{active}, 0
}

func (s *scriptedWindow) evaluations() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func TestPeakWindow_ActiveCachesForPollInterval(t *testing.T) {
	sw := &scriptedWindow{script: []bool{true, false}}
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	w := sw.window()
	w.Poll = 30 * time.Second
	w.Now = func() time.Time { return now }

	for range 3 {
		if active, err := w.Active(context.Background()); !active || err != nil {
			t.Fatalf("Active = %v, %v; want cached true", active, err)
		}
	}
	now = now.Add(30 * time.Second)
	if active, _ := w.Active(context.Background()); active {
		t.Fatal("Active = true after poll interval, want re-evaluated false")
	}
	if sw.evaluations() != 2 {
		t.Fatalf("evaluations = %d, want 2", sw.evaluations())
	}
}

func TestPeakWindow_ReturnsEvaluationError(t *testing.T) {
	sw := &scriptedWindow{err: errors.New("peak hours not found")}
	if _, err := sw.window().Active(context.Background()); err == nil {
		t.Fatal("Active error = nil, want the evaluation error")
	}
}

func TestPipelineSlotLease_ReleasesWhileHeldAndReacquires(t *testing.T) {
	slots := make(chan struct{}, 1)
	acquire := func(ctx context.Context) (func(), error) {
		select {
		case slots <- struct{}{}:
			return func() { <-slots }, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	release, _ := acquire(context.Background())
	lease := newPipelineSlotLease(acquire, release)

	lease.suspend()
	lease.suspend() // second concurrent waiter of the same pipeline
	if len(slots) != 0 {
		t.Fatal("slot still held while pipeline is held")
	}
	if err := lease.resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slots) != 0 {
		t.Fatal("slot re-acquired while another waiter is still held")
	}
	if err := lease.resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(slots) != 1 {
		t.Fatal("slot not re-acquired after last waiter resumed")
	}
	lease.Release()
	lease.Release()
	if len(slots) != 0 {
		t.Fatal("slot not released")
	}
}
