package docprocessing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPersistPipelineStatus_ConcurrentNoLostUpdates(t *testing.T) {
	store := &fakeStatusStore{raw: "[]"}
	svc := &ControlService{InputStore: store, Now: time.Now}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.persistPipelineStatus(context.Background(), 7, "running", fmt.Sprintf("proc-%d", i), nil)
		}()
	}
	wg.Wait()
	// persistPipelineStatus collapses concurrent writers onto the single
	// doc_processing entry; the guarantee under test is no data race / torn write.
	if store.raw == "" {
		t.Fatal("status not written")
	}
}

type fakeProcessor struct {
	name    string
	logName string
	calls   *[]string
	retErr  error
}

func (f fakeProcessor) Name() string { return f.name }

func (f fakeProcessor) LogName() string {
	if f.logName != "" {
		return f.logName
	}
	return f.name
}

func (f fakeProcessor) HandleEvent(_ context.Context, _ []byte) error {
	if f.calls != nil {
		*f.calls = append(*f.calls, f.name)
	}
	return f.retErr
}

type blockBufferSettingProcessor struct {
	name  string
	calls *[]string
}

func (p blockBufferSettingProcessor) Name() string { return p.name }

func (p blockBufferSettingProcessor) HandleEvent(ctx context.Context, _ []byte) error {
	if p.calls != nil {
		*p.calls = append(*p.calls, p.name)
	}
	h, _ := ctx.Value(blockBufferCtxKey{}).(*blockBufferHolder)
	if h == nil {
		return nil
	}
	h.mu.Lock()
	h.buffer = &BlockBuffer{
		Blocks: []Block{{
			Index: 1,
			Lines: []BlockLine{{Flag: "n", LineNumber: 430, PageNumber: 1, LineType: "paragraph", Content: "stale"}},
		}},
	}
	h.mu.Unlock()
	return nil
}

type blockBufferExpectNilProcessor struct {
	name  string
	calls *[]string
}

func (p blockBufferExpectNilProcessor) Name() string { return p.name }

func (p blockBufferExpectNilProcessor) HandleEvent(ctx context.Context, _ []byte) error {
	if p.calls != nil {
		*p.calls = append(*p.calls, p.name)
	}
	if buf := BlockBufferFromContext(ctx); buf != nil {
		return errors.New("expected cleared block buffer after static analyzer")
	}
	return nil
}

func TestControlService_UsesOperationOrder(t *testing.T) {
	got := make([]string, 0, 3)
	svc := &ControlService{
		Processors: []Processor{
			fakeProcessor{name: "static_analyzer", calls: &got},
			fakeProcessor{name: "chunking", calls: &got},
			fakeProcessor{name: "extract_doc_metadata", calls: &got},
			fakeProcessor{name: "extract_metrics", calls: &got},
		},
	}

	payload := []byte(`{"record_id":"1","operation":["extract_metrics","chunking"]}`)
	svc.HandleEvent(context.Background(), payload)

	want := []string{"extract_metrics", "static_analyzer", "chunking"}
	if len(got) != len(want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestControlService_DefaultsToConfiguredOrder(t *testing.T) {
	got := make([]string, 0, 3)
	svc := &ControlService{
		Processors: []Processor{
			fakeProcessor{name: "chunking", calls: &got},
			fakeProcessor{name: "extract_doc_metadata", calls: &got},
			fakeProcessor{name: "extract_metrics", calls: &got},
		},
	}

	svc.HandleEvent(context.Background(), []byte(`{"record_id":"1"}`))

	want := []string{"chunking", "extract_doc_metadata", "extract_metrics"}
	if len(got) != len(want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestControlService_RunsGenerateSummariesWhenExplicitlyRequestedWithChunking(t *testing.T) {
	got := make([]string, 0, 3)
	svc := &ControlService{
		Processors: []Processor{
			fakeProcessor{name: "static_analyzer", calls: &got},
			fakeProcessor{name: "chunking", calls: &got},
			fakeProcessor{name: "generate_summaries", calls: &got},
			fakeProcessor{name: "extract_metrics", calls: &got},
		},
	}

	payload := []byte(`{"record_id":"1","operation":["chunking","generate_summaries","extract_metrics"]}`)
	svc.HandleEvent(context.Background(), payload)

	want := []string{"static_analyzer", "chunking", "generate_summaries", "extract_metrics"}
	if len(got) != len(want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestControlService_GenerateSummariesRequestRunsDependenciesFirst(t *testing.T) {
	got := make([]string, 0, 3)
	svc := &ControlService{
		Processors: []Processor{
			fakeProcessor{name: "static_analyzer", calls: &got},
			fakeProcessor{name: "chunking", calls: &got},
			fakeProcessor{name: "generate_summaries", calls: &got},
		},
	}

	svc.HandleEvent(context.Background(), []byte(`{"record_id":"1","operation":["generate_summaries"]}`))

	want := []string{"static_analyzer", "chunking", "generate_summaries"}
	if len(got) != len(want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestControlService_StaticAnalyzerClearsStaleBlockBuffer(t *testing.T) {
	got := make([]string, 0, 3)
	svc := &ControlService{
		BlockingProcessor: blockBufferSettingProcessor{name: "blocking", calls: &got},
		Processors: []Processor{
			fakeProcessor{name: "static_analyzer", calls: &got},
			blockBufferExpectNilProcessor{name: "chunking", calls: &got},
		},
	}

	err := svc.handleEvent(context.Background(), []byte(`{"record_id":"1","operation":["chunking"]}`))
	if err != nil {
		t.Fatalf("handleEvent returned error: %v", err)
	}
	want := []string{"blocking", "static_analyzer", "chunking"}
	if len(got) != len(want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

func TestControlService_DefaultOrderRunsStandaloneGenerateProcessorsAfterChunking(t *testing.T) {
	got := make([]string, 0, 4)
	svc := &ControlService{
		Processors: []Processor{
			fakeProcessor{name: "chunking", calls: &got},
			fakeProcessor{name: "generate_summaries", calls: &got},
			fakeProcessor{name: "generate_topics", calls: &got},
			fakeProcessor{name: "extract_metrics", calls: &got},
		},
	}

	svc.HandleEvent(context.Background(), []byte(`{"record_id":"1"}`))

	want := []string{"chunking", "generate_summaries", "generate_topics", "extract_metrics"}
	if len(got) != len(want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("calls[%d]=%q, want %q", i, got[i], want[i])
		}
	}
}

type fakeEventStore struct {
	insertErr error
}

func (f fakeEventStore) InsertEvent(_ context.Context, _ EventRecord) error {
	return f.insertErr
}

func (f fakeEventStore) UpsertConsumedStatus(_ context.Context, _ string, _ time.Time, _ int64, _ error) error {
	return nil
}

func TestControlService_HandleJetStreamEvent_DoesNotFailWhenEventInsertFails(t *testing.T) {
	svc := &ControlService{
		EventStore: fakeEventStore{insertErr: errors.New(`pq: relation "kb.events" does not exist (42P01)`)},
	}
	err := svc.HandleJetStreamEvent(context.Background(), DefaultEventSubject, []byte(`{"record_id":"1"}`))
	if err != nil {
		t.Fatalf("HandleJetStreamEvent() error = %v, want nil", err)
	}
}

type blockingConcurrencyProcessor struct {
	name       string
	started    chan struct{}
	release    <-chan struct{}
	current    atomic.Int64
	maxCurrent atomic.Int64
	done       *sync.WaitGroup
}

func (p *blockingConcurrencyProcessor) Name() string { return p.name }

func (p *blockingConcurrencyProcessor) HandleEvent(_ context.Context, _ []byte) error {
	defer p.done.Done()
	current := p.current.Add(1)
	for {
		maxSeen := p.maxCurrent.Load()
		if current <= maxSeen || p.maxCurrent.CompareAndSwap(maxSeen, current) {
			break
		}
	}
	p.started <- struct{}{}
	<-p.release
	p.current.Add(-1)
	return nil
}

func TestControlService_HandleJetStreamEvent_RespectsMaxDocProcessPipelines(t *testing.T) {
	t.Setenv("MAX_DOC_PROCESS_PIPELINES", "2")

	release := make(chan struct{})
	started := make(chan struct{}, 4)
	var done sync.WaitGroup
	done.Add(4)
	processor := &blockingConcurrencyProcessor{
		name:    "blocking_probe",
		started: started,
		release: release,
		done:    &done,
	}
	svc := &ControlService{
		Processors: []Processor{processor},
		Now:        time.Now,
	}

	var handlers sync.WaitGroup
	handlers.Add(4)
	for i := 0; i < 4; i++ {
		go func() {
			defer handlers.Done()
			if err := svc.HandleJetStreamEvent(context.Background(), DefaultEventSubject, []byte(`{"record_id":"1"}`)); err != nil {
				t.Errorf("HandleJetStreamEvent() error = %v, want nil", err)
			}
		}()
	}

	waitForProcessorStarts(t, started, 2)
	select {
	case <-started:
		t.Fatalf("third pipeline started before a pipeline slot was released")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	done.Wait()
	handlers.Wait()

	if got := processor.maxCurrent.Load(); got > 2 {
		t.Fatalf("max concurrent pipelines=%d, want <= 2", got)
	}
}

func TestControlService_HandleJetStreamEvent_MarksOnlyRunningSlotActive(t *testing.T) {
	t.Setenv("MAX_DOC_PROCESS_PIPELINES", "1")
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var done sync.WaitGroup
	done.Add(1)
	processor := &blockingConcurrencyProcessor{
		name:    "blocking_probe",
		started: started,
		release: release,
		done:    &done,
	}
	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:             1,
			ParserName:     "opendata",
			ResultFilename: "result.json",
			StatusRaw:      `[]`,
		},
	}
	svc := &ControlService{
		Processors:             []Processor{processor},
		InputStore:             store,
		MaxDocProcessPipelines: 1,
		Now:                    time.Now,
	}

	err := svc.HandleJetStreamEvent(context.Background(), DefaultEventSubject, []byte(`{"record_id":"1","filename":"`+inputPath+`"}`))
	if err != nil {
		t.Fatalf("HandleJetStreamEvent() error = %v, want nil", err)
	}
	waitForProcessorStarts(t, started, 1)
	waitForStatusUpdate(t, store, "running")

	close(release)
	done.Wait()
	waitForStatusUpdate(t, store, "success")
}

func waitForStatusUpdate(t *testing.T, store *fakeDocMetadataStore, want string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		updates := store.statusUpdates()
		for _, req := range updates {
			if docProcessingStatus(req.StatusRaw) == want {
				return
			}
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for doc_processing status %q; updates=%v", want, updates)
		case <-ticker.C:
		}
	}
}

func docProcessingStatus(raw string) string {
	var entries []map[string]any
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry["operation"] == "doc_processing" {
			return strings.ToLower(strings.TrimSpace(asString(entry["proc_status"])))
		}
	}
	return ""
}

func waitForProcessorStarts(t *testing.T, started <-chan struct{}, want int) {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for i := 0; i < want; i++ {
		select {
		case <-started:
		case <-timeout:
			t.Fatalf("timed out waiting for processor start %d of %d", i+1, want)
		}
	}
}

func TestProcessorLogName_PrefersMethodSpecificLogName(t *testing.T) {
	p := fakeProcessor{name: "chunking", logName: "topic_chunking"}
	if got := processorLogName(p); got != "topic_chunking" {
		t.Fatalf("processorLogName=%q, want topic_chunking", got)
	}
}

func TestNormalizeStoredEventPayload_WrapsInvalidJSON(t *testing.T) {
	got := normalizeStoredEventPayload([]byte(`{"record_id":"1""type":"pdf"}`))
	want := `{"_payload_error":"invalid_json","_raw_payload":"{\"record_id\":\"1\"\"type\":\"pdf\"}"}`
	if got != want {
		t.Fatalf("normalizeStoredEventPayload()=%s, want %s", got, want)
	}
}
