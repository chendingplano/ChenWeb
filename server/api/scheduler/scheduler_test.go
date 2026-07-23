package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
)

type fakeStore struct {
	due          []Schedule
	starts       []startCall
	finishes     []finishCall
	advances     []advanceCall
	nextRunID    int64
	startErr     error
	finishErr    error
	advanceErr   error
}

type startCall struct {
	scheduleID int64
	jobType    string
}

type finishCall struct {
	runID  int64
	status string
	result map[string]any
	errMsg string
}

type advanceCall struct {
	scheduleID int64
	nextRunAt  time.Time
	enabled    bool
	status     string
}

func (s *fakeStore) DueSchedules(_ context.Context, _ time.Time, _ int) ([]Schedule, error) {
	return s.due, nil
}

func (s *fakeStore) StartRun(_ context.Context, scheduleID int64, jobType string) (int64, error) {
	if s.startErr != nil {
		return 0, s.startErr
	}
	s.starts = append(s.starts, startCall{scheduleID: scheduleID, jobType: jobType})
	s.nextRunID++
	return s.nextRunID, nil
}

func (s *fakeStore) FinishRun(_ context.Context, runID int64, status string, result map[string]any, runErr error) error {
	if s.finishErr != nil {
		return s.finishErr
	}
	msg := ""
	if runErr != nil {
		msg = runErr.Error()
	}
	s.finishes = append(s.finishes, finishCall{runID: runID, status: status, result: result, errMsg: msg})
	return nil
}

func (s *fakeStore) AdvanceSchedule(_ context.Context, scheduleID int64, nextRunAt time.Time, enabled bool, status string) error {
	if s.advanceErr != nil {
		return s.advanceErr
	}
	s.advances = append(s.advances, advanceCall{scheduleID: scheduleID, nextRunAt: nextRunAt, enabled: enabled, status: status})
	return nil
}

func TestRunDueSchedulesNoDueSchedulesIsANoOp(t *testing.T) {
	store := &fakeStore{}
	summary, err := RunDueSchedules(context.Background(), store, Registry{}, time.Now(), nil)
	if err != nil {
		t.Fatalf("RunDueSchedules: %v", err)
	}
	if summary.Scanned != 0 || len(store.starts) != 0 {
		t.Fatalf("summary = %+v, starts = %+v, want no-op", summary, store.starts)
	}
}

func TestRunDueSchedulesRunsKnownJobSuccessfully(t *testing.T) {
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	store := &fakeStore{due: []Schedule{
		{ID: 1, JobType: "widget_job", IntervalSeconds: 300, Params: map[string]any{"limit": float64(50)}},
	}}
	var gotParams map[string]any
	registry := Registry{"widget_job": JobDescriptor{Label: "Widget Job", Run: func(_ context.Context, params map[string]any, _ ApiTypes.JimoLogger) (map[string]any, error) {
		gotParams = params
		return map[string]any{"scanned": 5}, nil
	}}}

	summary, err := RunDueSchedules(context.Background(), store, registry, now, nil)
	if err != nil {
		t.Fatalf("RunDueSchedules: %v", err)
	}
	if summary.Scanned != 1 || summary.Succeeded != 1 || summary.Failed != 0 {
		t.Fatalf("summary = %+v, want 1 scanned/succeeded", summary)
	}
	if len(store.starts) != 1 || store.starts[0] != (startCall{scheduleID: 1, jobType: "widget_job"}) {
		t.Fatalf("starts = %+v", store.starts)
	}
	if gotParams["limit"] != float64(50) {
		t.Fatalf("job received params = %+v, want limit=50", gotParams)
	}
	if len(store.finishes) != 1 || store.finishes[0].status != "success" || store.finishes[0].result["scanned"] != 5 {
		t.Fatalf("finishes = %+v", store.finishes)
	}
	wantNext := now.Add(300 * time.Second)
	if len(store.advances) != 1 || !store.advances[0].nextRunAt.Equal(wantNext) || store.advances[0].status != "success" {
		t.Fatalf("advances = %+v, want next_run_at=%v status=success", store.advances, wantNext)
	}
	if !store.advances[0].enabled {
		t.Fatalf("advances = %+v, want a recurring schedule to stay enabled after running", store.advances)
	}
}

func TestRunDueSchedulesDisablesRunOnceScheduleAfterItRuns(t *testing.T) {
	now := time.Now()
	store := &fakeStore{due: []Schedule{
		{ID: 1, JobType: "widget_job", IntervalSeconds: 60, RunOnce: true},
	}}
	registry := Registry{"widget_job": JobDescriptor{Run: func(context.Context, map[string]any, ApiTypes.JimoLogger) (map[string]any, error) {
		return nil, nil
	}}}

	summary, err := RunDueSchedules(context.Background(), store, registry, now, nil)
	if err != nil {
		t.Fatalf("RunDueSchedules: %v", err)
	}
	if summary.Succeeded != 1 {
		t.Fatalf("summary = %+v, want 1 succeeded", summary)
	}
	if len(store.advances) != 1 || store.advances[0].enabled {
		t.Fatalf("advances = %+v, want a run-once schedule disabled after it runs so it is never picked up again", store.advances)
	}
}

func TestRunDueSchedulesDisablesRunOnceScheduleEvenWhenItFails(t *testing.T) {
	now := time.Now()
	store := &fakeStore{due: []Schedule{
		{ID: 1, JobType: "widget_job", IntervalSeconds: 60, RunOnce: true},
	}}
	registry := Registry{"widget_job": JobDescriptor{Run: func(context.Context, map[string]any, ApiTypes.JimoLogger) (map[string]any, error) {
		return nil, errors.New("boom")
	}}}

	_, err := RunDueSchedules(context.Background(), store, registry, now, nil)
	if err != nil {
		t.Fatalf("RunDueSchedules: %v", err)
	}
	if len(store.advances) != 1 || store.advances[0].enabled {
		t.Fatalf("advances = %+v, want a run-once schedule disabled even on failure — it must not retry on the next tick", store.advances)
	}
}

func TestRunDueSchedulesRecordsJobFailure(t *testing.T) {
	now := time.Now()
	store := &fakeStore{due: []Schedule{{ID: 1, JobType: "widget_job", IntervalSeconds: 60}}}
	registry := Registry{"widget_job": JobDescriptor{Run: func(_ context.Context, _ map[string]any, _ ApiTypes.JimoLogger) (map[string]any, error) {
		return nil, errors.New("boom")
	}}}

	summary, err := RunDueSchedules(context.Background(), store, registry, now, nil)
	if err != nil {
		t.Fatalf("RunDueSchedules: %v", err)
	}
	if summary.Failed != 1 || summary.Succeeded != 0 {
		t.Fatalf("summary = %+v, want 1 failed", summary)
	}
	if len(store.finishes) != 1 || store.finishes[0].status != "failed" || store.finishes[0].errMsg != "boom" {
		t.Fatalf("finishes = %+v", store.finishes)
	}
	if len(store.advances) != 1 || store.advances[0].status != "failed" {
		t.Fatalf("advances = %+v, want status=failed (still rescheduled, not stuck)", store.advances)
	}
}

func TestRunDueSchedulesRecordsUnknownJobTypeAsFailedRunNotSilentSkip(t *testing.T) {
	now := time.Now()
	store := &fakeStore{due: []Schedule{{ID: 1, JobType: "no_such_job", IntervalSeconds: 60}}}

	summary, err := RunDueSchedules(context.Background(), store, Registry{}, now, nil)
	if err != nil {
		t.Fatalf("RunDueSchedules: %v", err)
	}
	if summary.Failed != 1 {
		t.Fatalf("summary = %+v, want 1 failed", summary)
	}
	if len(store.starts) != 1 || len(store.finishes) != 1 || store.finishes[0].status != "failed" {
		t.Fatalf("starts = %+v, finishes = %+v, want a recorded failed run for observability, not a silent skip", store.starts, store.finishes)
	}
	if len(store.advances) != 1 {
		t.Fatalf("advances = %+v, want the schedule still rescheduled so it doesn't spin every tick", store.advances)
	}
}

func TestRunDueSchedulesProcessesEachScheduleIndependently(t *testing.T) {
	now := time.Now()
	store := &fakeStore{due: []Schedule{
		{ID: 1, JobType: "ok_job", IntervalSeconds: 60},
		{ID: 2, JobType: "bad_job", IntervalSeconds: 60},
	}}
	registry := Registry{
		"ok_job":  JobDescriptor{Run: func(context.Context, map[string]any, ApiTypes.JimoLogger) (map[string]any, error) { return nil, nil }},
		"bad_job": JobDescriptor{Run: func(context.Context, map[string]any, ApiTypes.JimoLogger) (map[string]any, error) { return nil, errors.New("bad") }},
	}
	summary, err := RunDueSchedules(context.Background(), store, registry, now, nil)
	if err != nil {
		t.Fatalf("RunDueSchedules: %v", err)
	}
	if summary.Scanned != 2 || summary.Succeeded != 1 || summary.Failed != 1 {
		t.Fatalf("summary = %+v, want one success and one failure, both processed", summary)
	}
}
