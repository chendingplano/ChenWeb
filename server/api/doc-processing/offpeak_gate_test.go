package docprocessing

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// scriptedGate returns a gate whose window reports held for the first
// heldChecks evaluations and off-peak afterwards.
func scriptedGate(heldChecks int32, evalErr error) (*OffPeakGate, *int32) {
	var calls int32
	g := &OffPeakGate{WindowName: "test window", Poll: time.Millisecond}
	g.peakSoon = func(context.Context, time.Time, time.Duration) (bool, error) {
		n := atomic.AddInt32(&calls, 1)
		if evalErr != nil {
			return false, evalErr
		}
		return n <= heldChecks, nil
	}
	return g, &calls
}

func TestOffPeakGate_HeldNowCachesForPollInterval(t *testing.T) {
	g, calls := scriptedGate(100, nil)
	g.Poll = time.Hour
	now := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	g.Now = func() time.Time { return now }

	if !g.HeldNow(context.Background()) || !g.HeldNow(context.Background()) {
		t.Fatal("HeldNow = false, want true")
	}
	if *calls != 1 {
		t.Fatalf("evaluations = %d, want 1 (cached)", *calls)
	}
	now = now.Add(time.Hour)
	g.HeldNow(context.Background())
	if *calls != 2 {
		t.Fatalf("evaluations = %d, want 2 after poll interval", *calls)
	}
}

func TestOffPeakGate_EvaluationErrorFailsOpen(t *testing.T) {
	g, _ := scriptedGate(0, errors.New("peak hours record not found"))
	if g.HeldNow(context.Background()) {
		t.Fatal("HeldNow = true, want false on evaluation error")
	}
}

func TestOffPeakGate_WaitResumesWhenOffPeak(t *testing.T) {
	g, calls := scriptedGate(3, nil)
	holds := 0
	waited, err := g.Wait(context.Background(), func() { holds++ })
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if !waited || holds != 1 {
		t.Fatalf("waited=%v holds=%d, want true/1", waited, holds)
	}
	if *calls != 4 {
		t.Fatalf("evaluations = %d, want 4", *calls)
	}
}

func TestOffPeakGate_WaitReturnsStopCause(t *testing.T) {
	g, _ := scriptedGate(1<<30, nil)
	ctx, cancel := context.WithCancelCause(context.Background())
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel(ErrPipelineStopped)
	}()
	_, err := g.Wait(ctx, nil)
	if !errors.Is(err, ErrPipelineStopped) {
		t.Fatalf("err = %v, want ErrPipelineStopped", err)
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

func TestRunSingleProcessorCollect_HoldsOffPeakPipeline(t *testing.T) {
	g, calls := scriptedGate(2, nil)
	s := &ControlService{OffPeak: g}
	var ran []string
	p := fakeProcessor{name: "extract_metrics", calls: &ran}

	res := s.runSingleProcessorCollect(withOffPeakHold(context.Background()), nil, p, 1)
	if res.failed || len(ran) != 1 {
		t.Fatalf("res=%+v ran=%v, want processor run after hold", res, ran)
	}
	if *calls != 3 {
		t.Fatalf("evaluations = %d, want 3 (held twice, then off-peak)", *calls)
	}
}

func TestRunSingleProcessorCollect_NoHoldWithoutOffPeakMode(t *testing.T) {
	g, calls := scriptedGate(1<<30, nil)
	s := &ControlService{OffPeak: g}
	var ran []string

	s.runSingleProcessorCollect(context.Background(), nil, fakeProcessor{name: "extract_metrics", calls: &ran}, 1)
	s.runSingleProcessorCollect(withOffPeakHold(context.Background()), nil, fakeProcessor{name: "blocking", calls: &ran}, 1)
	if len(ran) != 2 || *calls != 0 {
		t.Fatalf("ran=%v evaluations=%d, want both run without evaluation", ran, *calls)
	}
}

func TestRunSingleProcessorCollect_StopWhileHeld(t *testing.T) {
	g, _ := scriptedGate(1<<30, nil)
	s := &ControlService{OffPeak: g}
	var ran []string
	ctx, cancel := context.WithCancelCause(withOffPeakHold(context.Background()))
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel(ErrPipelineStopped)
	}()

	res := s.runSingleProcessorCollect(ctx, nil, fakeProcessor{name: "extract_metrics", calls: &ran}, 1)
	if !res.stopped || len(ran) != 0 {
		t.Fatalf("res=%+v ran=%v, want stopped without running", res, ran)
	}
}
