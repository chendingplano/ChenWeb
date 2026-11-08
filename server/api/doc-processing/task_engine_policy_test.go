package docprocessing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sharedllm "github.com/chendingplano/shared/go/api/llm"
)

// modeStore adds a processing_mode to fakeDocMetadataStore.
type modeStore struct {
	*fakeDocMetadataStore
	mode string
	err  error
}

func (s modeStore) GetProcessingMode(context.Context, int64) (string, error) {
	return s.mode, s.err
}

func (s modeStore) statusRaw() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rec.StatusRaw
}

type memStopStore struct {
	mu        sync.Mutex
	requested bool
}

func (m *memStopStore) SetStopRequested(context.Context, int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requested = true
	return nil
}

func (m *memStopStore) IsStopRequested(context.Context, int64) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.requested, nil
}

func (m *memStopStore) ClearStopRequested(context.Context, int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requested = false
	return nil
}

func (m *memStopStore) MarkDocProcessingStopped(context.Context, int64) error { return nil }

type lifecycleSink struct {
	mu      sync.Mutex
	records []sharedllm.UsageCaptureRecord
	err     error
}

func (s *lifecycleSink) Capture(_ context.Context, rec sharedllm.UsageCaptureRecord) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, rec)
	return "evt", s.err
}

func (s *lifecycleSink) snapshot() []sharedllm.UsageCaptureRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sharedllm.UsageCaptureRecord(nil), s.records...)
}

// ctxProcessor blocks until its context ends and returns the cause.
type ctxProcessor struct {
	name    string
	started chan struct{}
}

func (p ctxProcessor) Name() string { return p.name }

func (p ctxProcessor) HandleEvent(ctx context.Context, _ []byte) error {
	close(p.started)
	<-ctx.Done()
	return context.Cause(ctx)
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// peakTestTask returns a store and event payload that pass preflight.
func peakTestTask(t *testing.T, mode string) (modeStore, []byte) {
	t.Helper()
	inputPath := filepath.Join(t.TempDir(), "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := modeStore{fakeDocMetadataStore: &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID: 7, ParserName: "opendata", ResultFilename: "result.json", StagingFilename: inputPath,
		StatusRaw: "[]", InputDocType: "pdf", SourceLanguage: "en",
	}}, mode: mode}
	payload := []byte(`{"user_id":"u1","record_id":"7","filename":"` + inputPath + `","operation":["extract_metrics"]}`)
	return store, payload
}

func TestTaskEnginePolicy_ModeFor(t *testing.T) {
	p := taskEnginePolicy{DefaultMode: PeakModeAutoPeak}
	if got := p.modeFor(PeakModeAutoOffPeak); got != PeakModeAutoOffPeak {
		t.Fatalf("task mode: got %q", got)
	}
	if got := p.modeFor(""); got != PeakModeAutoPeak {
		t.Fatalf("engine default: got %q", got)
	}
	if got := (taskEnginePolicy{}).modeFor(""); got != PeakModeNone {
		t.Fatalf("zero policy: got %q", got)
	}
}

func TestTaskEnginePolicy_Permitted(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   string
		active bool
		err    error
		want   bool
	}{
		{"none during peak", PeakModeNone, true, nil, true},
		{"offpeak during peak", PeakModeAutoOffPeak, true, nil, false},
		{"offpeak off-peak", PeakModeAutoOffPeak, false, nil, true},
		{"peak during peak", PeakModeAutoPeak, true, nil, true},
		{"peak off-peak", PeakModeAutoPeak, false, nil, false},
		{"offpeak evaluation error", PeakModeAutoOffPeak, false, errors.New("missing record"), true},
		{"peak evaluation error", PeakModeAutoPeak, false, errors.New("missing record"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sw := &scriptedWindow{script: []bool{tc.active}, err: tc.err}
			p := taskEnginePolicy{Window: sw.window()}
			if got := p.permitted(context.Background(), tc.mode); got != tc.want {
				t.Fatalf("permitted = %v, want %v", got, tc.want)
			}
		})
	}
	if !(taskEnginePolicy{}).permitted(context.Background(), PeakModeAutoOffPeak) {
		t.Fatal("no window must permit work")
	}
}

func TestTaskPeakMode(t *testing.T) {
	window := (&scriptedWindow{script: []bool{false}}).window()
	for _, tc := range []struct {
		store DocMetadataStore
		want  string
	}{
		{modeStore{mode: ProcessingModeAutoOffPeak}, PeakModeAutoOffPeak},
		{modeStore{mode: "auto"}, ""},
		{modeStore{err: errors.New("db unavailable")}, ""},
		{&fakeDocMetadataStore{}, ""},
	} {
		s := &ControlService{InputStore: tc.store, TaskPolicy: taskEnginePolicy{Window: window}}
		if got := s.taskPeakMode(context.Background(), 1); got != tc.want {
			t.Fatalf("%T: got %q, want %q", tc.store, got, tc.want)
		}
	}
	s := &ControlService{InputStore: modeStore{mode: ProcessingModeAutoOffPeak}}
	if got := s.taskPeakMode(context.Background(), 1); got != "" {
		t.Fatalf("no window: got %q, want no lookup", got)
	}
}

func TestAdmitTask_WaitsWithoutSlotAndShowsWaiting(t *testing.T) {
	sw := &scriptedWindow{script: []bool{true}}
	store, _ := peakTestTask(t, "")
	s := &ControlService{InputStore: store, MaxDocProcessPipelines: 1, TaskPolicy: taskEnginePolicy{Window: sw.window()}}
	release, err := s.acquirePipelineSlot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	lease := newPipelineSlotLease(s.acquirePipelineSlot, release)
	defer lease.Release()

	done := make(chan error, 1)
	go func() {
		done <- s.admitTask(withPipelineSlotLease(context.Background(), lease), 7, PeakModeAutoOffPeak)
	}()
	waitFor(t, "waiting status", func() bool {
		return strings.Contains(store.statusRaw(), `"progress":"waiting for off-peak hours (test window)"`)
	})
	other, err := s.acquirePipelineSlot(context.Background()) // a plain task is not blocked
	if err != nil {
		t.Fatal(err)
	}
	other()
	select {
	case err := <-done:
		t.Fatalf("admitted during peak: %v", err)
	default:
	}

	sw.set(false)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(s.pipelineSlots) != 1 {
		t.Fatal("slot not re-acquired before starting")
	}
}

func TestAdmitTask_RechecksAfterReacquiringSlot(t *testing.T) {
	// The span opens, closes again while the task waits for its slot, then
	// opens for good.
	sw := &scriptedWindow{script: []bool{true, false, true, false}}
	w := sw.window()
	w.Poll = time.Nanosecond
	s := &ControlService{TaskPolicy: taskEnginePolicy{Window: w}}
	if err := s.admitTask(context.Background(), 7, PeakModeAutoOffPeak); err != nil {
		t.Fatal(err)
	}
	if got := sw.evaluations(); got != 5 {
		t.Fatalf("evaluations = %d, want 5 (wait, open, closed again, open, final check)", got)
	}
}

func TestAdmitTask_AutoPeakWaitsForPeak(t *testing.T) {
	sw := &scriptedWindow{script: []bool{false, false, true}}
	s := &ControlService{TaskPolicy: taskEnginePolicy{Window: sw.window()}}
	if err := s.admitTask(context.Background(), 7, PeakModeAutoPeak); err != nil {
		t.Fatal(err)
	}
	if sw.evaluations() < 3 {
		t.Fatalf("evaluations = %d, want the task to wait for peak", sw.evaluations())
	}
}

func TestHandleEvent_StopWhileWaiting(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	sw := &scriptedWindow{script: []bool{true}}
	store, payload := peakTestTask(t, ProcessingModeAutoOffPeak)
	stops := &memStopStore{}
	runs := &fakeRunStore{}
	var ran []string
	s := &ControlService{
		InputStore: store, StopStore: stops, RunStore: runs,
		Processors: []Processor{fakeProcessor{name: "extract_metrics", calls: &ran}},
		TaskPolicy: taskEnginePolicy{DefaultMode: PeakModeNone, Window: sw.window()},
	}

	done := make(chan error, 1)
	go func() { done <- s.handleEvent(context.Background(), payload) }()
	waitFor(t, "waiting status", func() bool { return strings.Contains(store.statusRaw(), "waiting for off-peak hours") })
	_ = stops.SetStopRequested(context.Background(), 7)

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(ran) != 0 || len(runs.creates) != 0 {
		t.Fatalf("ran=%v runs=%d, want nothing started", ran, len(runs.creates))
	}
	if raw := store.statusRaw(); !strings.Contains(raw, `"proc_status":"stopped"`) || strings.Contains(raw, "waiting for") {
		t.Fatalf("status = %s, want stopped without the waiting note", raw)
	}
	if requested, _ := stops.IsStopRequested(context.Background(), 7); requested {
		t.Fatal("stop flag not cleared")
	}
}

func TestHandleEvent_ModeNoneIgnoresPeak(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	sw := &scriptedWindow{script: []bool{true}}
	store, payload := peakTestTask(t, "auto")
	var ran []string
	s := &ControlService{
		InputStore: store,
		Processors: []Processor{fakeProcessor{name: "extract_metrics", calls: &ran}},
		TaskPolicy: taskEnginePolicy{DefaultMode: PeakModeNone, CrossPeak: CrossPeakAbort, Window: sw.window()},
	}
	if err := s.handleEvent(context.Background(), payload); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 || sw.evaluations() != 0 {
		t.Fatalf("ran=%v evaluations=%d, want run without peak evaluation", ran, sw.evaluations())
	}
}

func TestHandleEvent_AbortAtBoundary(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	sw := &scriptedWindow{script: []bool{false}}
	store, payload := peakTestTask(t, "auto")
	runs := &fakeRunStore{}
	p := ctxProcessor{name: "extract_metrics", started: make(chan struct{})}
	s := &ControlService{
		InputStore: store, RunStore: runs, Processors: []Processor{p},
		TaskPolicy: taskEnginePolicy{DefaultMode: PeakModeAutoOffPeak, CrossPeak: CrossPeakAbort, Window: sw.window()},
	}

	done := make(chan error, 1)
	go func() { done <- s.handleEvent(context.Background(), payload) }()
	<-p.started
	sw.set(true) // peak begins while the processor runs

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(runs.closes) != 1 || runs.closes[0].status != "stopped" || runs.closes[0].errMsg == nil ||
		*runs.closes[0].errMsg != "aborted at peak-hour boundary" {
		t.Fatalf("run closes = %+v, want stopped with the abort reason", runs.closes)
	}
	raw := store.statusRaw()
	if !strings.Contains(raw, `"proc_status":"stopped"`) || !strings.Contains(raw, "aborted at peak-hour boundary") {
		t.Fatalf("status = %s, want stopped with the abort reason", raw)
	}
}

// phaseCProcessor finishes Phase B at once, then blocks in Phase C until its
// context ends.
type phaseCProcessor struct {
	name    string
	started chan struct{}
}

func (p phaseCProcessor) Name() string                              { return p.name }
func (p phaseCProcessor) HandleEvent(context.Context, []byte) error { return nil }
func (p phaseCProcessor) PostProcessIndex(ctx context.Context, _ int64) error {
	close(p.started)
	<-ctx.Done()
	return context.Cause(ctx)
}

func TestHandleEvent_AbortDuringPhaseC(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	sw := &scriptedWindow{script: []bool{false}}
	store, payload := peakTestTask(t, "auto")
	runs := &fakeRunStore{}
	p := phaseCProcessor{name: "extract_metrics", started: make(chan struct{})}
	s := &ControlService{
		InputStore: store, RunStore: runs, Processors: []Processor{p},
		TaskPolicy: taskEnginePolicy{DefaultMode: PeakModeAutoOffPeak, CrossPeak: CrossPeakAbort, Window: sw.window()},
	}

	done := make(chan error, 1)
	go func() { done <- s.handleEvent(context.Background(), payload) }()
	<-p.started
	sw.set(true)

	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if len(runs.closes) != 1 || runs.closes[0].status != "stopped" {
		t.Fatalf("run closes = %+v, want stopped", runs.closes)
	}
}

func TestApplyCrossPeakRule_NoGateForIgnoreOrModeNone(t *testing.T) {
	window := (&scriptedWindow{script: []bool{true}}).window()
	ctx := context.Background()
	for _, tc := range []struct{ rule, mode string }{
		{CrossPeakIgnore, PeakModeAutoOffPeak},
		{CrossPeakPauseLLM, PeakModeNone},
		{CrossPeakAbort, PeakModeNone},
	} {
		s := &ControlService{TaskPolicy: taskEnginePolicy{CrossPeak: tc.rule, Window: window}}
		if got := s.applyCrossPeakRule(ctx, func(error) { t.Fatal("cancelled") }, 7, tc.mode); got != ctx {
			t.Fatalf("%s/%s changed the context", tc.rule, tc.mode)
		}
	}
}

// pauseLLMTask returns a task context with the pause-llm gate armed, a real
// OpenAIJSONClient whose server counts requests, and the lifecycle sink.
func pauseLLMTask(t *testing.T, sw *scriptedWindow, sinkErr error) (context.Context, *sharedllm.OpenAIJSONClient, *atomic.Int32, *lifecycleSink) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(srv.Close)
	sink := &lifecycleSink{err: sinkErr}
	s := &ControlService{TaskPolicy: taskEnginePolicy{CrossPeak: CrossPeakPauseLLM, Window: sw.window(), Events: sink}}
	ctx := withLLMRunID(withRunID(withLLMUserID(context.Background(), "u1"), 9), 9009)
	ctx = s.applyCrossPeakRule(ctx, nil, 7, PeakModeAutoOffPeak)
	client := &sharedllm.OpenAIJSONClient{BaseURL: srv.URL, APIKey: "k", ModelName: "deepseek-test", HTTPClient: srv.Client()}
	return ctx, client, &hits, sink
}

func extractInput() sharedllm.JSONExtractionInput {
	return sharedllm.JSONExtractionInput{PromptName: "p", PromptText: "prompt", InputText: "text", CallReason: "extract", CallLoc: "LOC_T"}
}

func TestPauseLLM_HoldsRealRequestAndRecordsPairedEvents(t *testing.T) {
	sw := &scriptedWindow{script: []bool{true}}
	ctx, client, hits, sink := pauseLLMTask(t, sw, nil)

	done := make(chan error, 1)
	go func() {
		_, err := client.ExtractText(ctx, extractInput())
		done <- err
	}()
	waitFor(t, "llm_hold row", func() bool { return len(sink.snapshot()) == 1 })
	time.Sleep(5 * time.Millisecond)
	if hits.Load() != 0 {
		t.Fatal("provider request sent during the disallowed span")
	}
	sw.set(false)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 {
		t.Fatalf("requests = %d, want 1", hits.Load())
	}

	rows := sink.snapshot()
	if len(rows) != 2 || rows[0].EventKind != llmEventKindHold || rows[1].EventKind != llmEventKindResume {
		t.Fatalf("rows = %+v, want llm_hold then llm_resume", rows)
	}
	for _, r := range rows {
		if r.UserID != "u1" || r.RecordID != 7 || r.RunID != 9009 || r.ModelName != "deepseek-test" ||
			r.PromptName != "p" || r.CallLoc != "LOC_T" || r.TotalTokens != 0 ||
			r.Metadata["peak_mode"] != PeakModeAutoOffPeak || r.Metadata["cross_peak_mode"] != CrossPeakPauseLLM {
			t.Fatalf("row attribution = %+v", r)
		}
	}
	if rows[0].Metadata["hold_id"] == "" || rows[0].Metadata["hold_id"] != rows[1].Metadata["hold_id"] {
		t.Fatalf("hold ids = %v / %v, want one shared id", rows[0].Metadata["hold_id"], rows[1].Metadata["hold_id"])
	}
	if _, ok := rows[1].Metadata["held_ms"]; !ok {
		t.Fatal("resume row has no held_ms")
	}
}

func TestPauseLLM_StopWhileHeldSendsNothing(t *testing.T) {
	sw := &scriptedWindow{script: []bool{true}}
	ctx, client, hits, sink := pauseLLMTask(t, sw, nil)
	ctx, cancel := context.WithCancelCause(ctx)

	done := make(chan error, 1)
	go func() {
		_, err := client.ExtractText(ctx, extractInput())
		done <- err
	}()
	waitFor(t, "llm_hold row", func() bool { return len(sink.snapshot()) == 1 })
	cancel(ErrPipelineStopped)

	if err := <-done; !errors.Is(err, ErrPipelineStopped) {
		t.Fatalf("err = %v, want the stop cause", err)
	}
	rows := sink.snapshot()
	if hits.Load() != 0 || len(rows) != 2 || rows[1].EventKind != llmEventKindHoldCancelled || rows[1].ErrorMessage == "" {
		t.Fatalf("requests=%d rows=%+v, want no request and llm_hold_cancelled", hits.Load(), rows)
	}
}

func TestPauseLLM_EventWriteFailureDoesNotBlockCall(t *testing.T) {
	sw := &scriptedWindow{script: []bool{true}}
	ctx, client, hits, sink := pauseLLMTask(t, sw, errors.New("db down"))

	done := make(chan error, 1)
	go func() {
		_, err := client.ExtractText(ctx, extractInput())
		done <- err
	}()
	waitFor(t, "attempted llm_hold row", func() bool { return len(sink.snapshot()) == 1 })
	sw.set(false)
	if err := <-done; err != nil || hits.Load() != 1 {
		t.Fatalf("err=%v requests=%d, want the call held and then sent", err, hits.Load())
	}
}

func TestPauseLLM_PermittedCallIsNotHeld(t *testing.T) {
	sw := &scriptedWindow{script: []bool{false}}
	ctx, client, hits, sink := pauseLLMTask(t, sw, nil)
	if _, err := client.ExtractText(ctx, extractInput()); err != nil {
		t.Fatal(err)
	}
	if hits.Load() != 1 || len(sink.snapshot()) != 0 {
		t.Fatalf("requests=%d rows=%d, want the request sent with no hold rows", hits.Load(), len(sink.snapshot()))
	}
}
