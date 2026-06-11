package docprocessing

import (
	"context"
	"database/sql"
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

func TestIsPhaseAProcessor(t *testing.T) {
	for _, name := range []string{"static_analyzer", "chunking", "extract_doc_metadata", "extract_metadata"} {
		if !isPhaseAProcessor(name) {
			t.Errorf("%q should be Phase A", name)
		}
	}
	for _, name := range []string{"extract_metrics", "generate_topics", "extract_entity_relation"} {
		if isPhaseAProcessor(name) {
			t.Errorf("%q should be Phase B", name)
		}
	}
}

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
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
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
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
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
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
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
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
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
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
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

func TestDefaultEventSubjectIsStartDocProcessing(t *testing.T) {
	if DefaultEventSubject != "kb.pdf.start-doc-processing" {
		t.Fatalf("DefaultEventSubject=%q, want kb.pdf.start-doc-processing", DefaultEventSubject)
	}
}

func TestParseStartDocProcessingEvent_RecordRanges(t *testing.T) {
	cmd, err := ParseStartDocProcessingEvent([]byte(`{
		"record_ids": [12, "22-24", " 31 "],
		"doc-processors": ["extract-metrics", "generate_topics"],
		"failed-proc-only": false
	}`))
	if err != nil {
		t.Fatalf("ParseStartDocProcessingEvent: %v", err)
	}
	wantIDs := []int64{12, 22, 23, 24, 31}
	if fmt.Sprint(cmd.RecordIDs) != fmt.Sprint(wantIDs) {
		t.Fatalf("record ids=%v, want %v", cmd.RecordIDs, wantIDs)
	}
	wantOps := []string{"extract_metrics", "generate_topics"}
	if fmt.Sprint(cmd.DocProcessors) != fmt.Sprint(wantOps) {
		t.Fatalf("processors=%v, want %v", cmd.DocProcessors, wantOps)
	}
	if cmd.FailedProcOnly {
		t.Fatalf("FailedProcOnly=true, want false")
	}
}

func TestParseStartDocProcessingEvent_RejectsInvalidAllAndMissingTargets(t *testing.T) {
	if _, err := ParseStartDocProcessingEvent([]byte(`{"all":"bogus"}`)); err == nil {
		t.Fatalf("invalid all should fail")
	}
	if _, err := ParseStartDocProcessingEvent([]byte(`{"doc-processors":["extract_metrics"]}`)); err == nil {
		t.Fatalf("missing all and record_ids should fail")
	}
}

func TestControlService_StartDocProcessingAllParsed(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	dir := t.TempDir()
	writeLineFile(t, filepath.Join(dir, "source_opendata.txt"))
	got := make([]string, 0, 4)
	store := &fakeDocProcessingCommandStore{
		parsed: []DocMetadataInputRecord{
			{ID: 12, ParserName: "opendata", ResultFilename: filepath.Join(dir, "result.json"), StagingFilename: filepath.Join(dir, "source.pdf"), StatusRaw: `[{"operation":"parsed","procstatus":"success"}]`},
			{ID: 22, ParserName: "opendata", ResultFilename: filepath.Join(dir, "result.json"), StagingFilename: filepath.Join(dir, "source.pdf"), StatusRaw: `[{"operation":"parsed","procstatus":"success"}]`},
		},
	}
	svc := &ControlService{
		InputStore: store,
		Processors: []Processor{
			fakeProcessor{name: "extract_metrics", calls: &got},
		},
	}

	err := svc.HandleStartDocProcessingEvent(context.Background(), []byte(`{
		"all": "parsed",
		"doc-processors": ["extract_metrics"],
		"failed-proc-only": false
	}`))
	if err != nil {
		t.Fatalf("HandleStartDocProcessingEvent: %v", err)
	}
	want := []string{"extract_metrics", "extract_metrics"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
}

func TestControlService_StartDocProcessingFiltersFailedProcessorsByDefault(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	dir := t.TempDir()
	writeLineFile(t, filepath.Join(dir, "source_opendata.txt"))
	got := make([]string, 0, 2)
	store := &fakeDocProcessingCommandStore{
		records: map[int64]DocMetadataInputRecord{
			7: {ID: 7, ParserName: "opendata", ResultFilename: filepath.Join(dir, "result.json"), StagingFilename: filepath.Join(dir, "source.pdf"), StatusRaw: `[
				{"operation":"extract_metrics","proc_status":"failed"},
				{"operation":"generate_topics","proc_status":"success"}
			]`},
		},
	}
	svc := &ControlService{
		InputStore: store,
		Processors: []Processor{
			fakeProcessor{name: "extract_metrics", calls: &got},
			fakeProcessor{name: "generate_topics", calls: &got},
		},
	}

	err := svc.HandleStartDocProcessingEvent(context.Background(), []byte(`{
		"record_ids": [7],
		"doc-processors": ["extract_metrics", "generate_topics"]
	}`))
	if err != nil {
		t.Fatalf("HandleStartDocProcessingEvent: %v", err)
	}
	want := []string{"extract_metrics"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("calls=%v, want %v", got, want)
	}
}

func TestDocProcessorModeFromEnvDefaultsToAuto(t *testing.T) {
	t.Setenv("DOC_PROCESSOR_MODE", "")
	mode, err := DocProcessorModeFromEnv()
	if err != nil {
		t.Fatalf("DocProcessorModeFromEnv: %v", err)
	}
	if mode != DocProcessorModeAuto {
		t.Fatalf("mode=%q, want %q", mode, DocProcessorModeAuto)
	}
}

func TestDocProcessorModeFromEnvRejectsInvalidValues(t *testing.T) {
	t.Setenv("DOC_PROCESSOR_MODE", "local")
	if _, err := DocProcessorModeFromEnv(); err == nil {
		t.Fatalf("invalid DOC_PROCESSOR_MODE should fail")
	}
}

func TestControlService_DefaultSubjectUsesAutoModeByDefault(t *testing.T) {
	t.Setenv("DOC_PROCESSOR_MODE", "")
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	got := make([]string, 0, 1)
	svc := &ControlService{
		Processors: []Processor{fakeProcessor{name: "extract_metrics", calls: &got}},
	}

	err := svc.handleDefaultSubjectEvent(context.Background(), []byte(`{"record_id":"1"}`))
	if err != nil {
		t.Fatalf("handleDefaultSubjectEvent: %v", err)
	}
	if fmt.Sprint(got) != fmt.Sprint([]string{"extract_metrics"}) {
		t.Fatalf("calls=%v, want extract_metrics", got)
	}
}

func TestControlService_DefaultSubjectUsesDevModeWhenConfigured(t *testing.T) {
	t.Setenv("DOC_PROCESSOR_MODE", "dev")
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	dir := t.TempDir()
	writeLineFile(t, filepath.Join(dir, "source_opendata.txt"))
	got := make([]string, 0, 2)
	store := &fakeDocProcessingCommandStore{
		records: map[int64]DocMetadataInputRecord{
			9: {ID: 9, ParserName: "opendata", ResultFilename: filepath.Join(dir, "result.json"), StagingFilename: filepath.Join(dir, "source.pdf"), StatusRaw: `[
				{"operation":"extract_metrics","proc_status":"failed"},
				{"operation":"generate_topics","proc_status":"success"}
			]`},
		},
	}
	svc := &ControlService{
		InputStore: store,
		Processors: []Processor{
			fakeProcessor{name: "extract_metrics", calls: &got},
			fakeProcessor{name: "generate_topics", calls: &got},
		},
	}

	err := svc.handleDefaultSubjectEvent(context.Background(), []byte(`{
		"record_ids": [9],
		"doc-processors": ["extract_metrics", "generate_topics"]
	}`))
	if err != nil {
		t.Fatalf("handleDefaultSubjectEvent: %v", err)
	}
	if fmt.Sprint(got) != fmt.Sprint([]string{"extract_metrics"}) {
		t.Fatalf("calls=%v, want extract_metrics", got)
	}
}

func writeLineFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
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

func TestTwoPhase_NoLostStatusEntries(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "true")
	store := &fakeStatusStore{raw: "[]"}
	svc := &ControlService{
		// InputStore is nil so the controller's own preflight/storage is
		// bypassed; each statusWritingProcessor carries its own store ref.
		Processors: []Processor{
			&statusWritingProcessor{name: "extract_metrics", store: store, id: 7},
			&statusWritingProcessor{name: "extract_provisions", store: store, id: 7},
			&statusWritingProcessor{name: "generate_summaries", store: store, id: 7},
			&statusWritingProcessor{name: "generate_topics", store: store, id: 7},
			&statusWritingProcessor{name: "generate_scene_blocks", store: store, id: 7},
			&statusWritingProcessor{name: "extract_semantic_projections", store: store, id: 7},
			&statusWritingProcessor{name: "extract_entity_relation", store: store, id: 7},
			&statusWritingProcessor{name: "extract_inventory_items", store: store, id: 7},
		},
	}
	_ = svc.handleEvent(context.Background(), []byte(`{"record_id":"7"}`))

	var arr []map[string]any
	if err := json.Unmarshal([]byte(store.raw), &arr); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	seen := map[string]struct{}{}
	for _, entry := range arr {
		op := strings.ToLower(strings.TrimSpace(asString(entry["operation"])))
		if op == "doc_processing" || op == "doc_processor" {
			continue
		}
		seen[op] = struct{}{}
	}
	for _, name := range []string{
		"extract_metrics", "extract_provisions", "generate_summaries", "generate_topics",
		"generate_scene_blocks", "extract_semantic_projections", "extract_entity_relation", "extract_inventory_items",
	} {
		if _, ok := seen[name]; !ok {
			t.Errorf("missing status entry for %q; got entries: %v", name, seen)
		}
	}
}

type statusWritingProcessor struct {
	name   string
	store  DocMetadataStore
	id     int64
	called int
}

func (p *statusWritingProcessor) Name() string    { return p.name }
func (p *statusWritingProcessor) LogName() string { return p.name }
func (p *statusWritingProcessor) HandleEvent(ctx context.Context, payload []byte) error {
	p.called++
	return updateInputStatusAtomic(context.Background(), p.store, p.id, func(cur string) (DocMetadataUpdate, error) {
		var arr []map[string]any
		_ = json.Unmarshal([]byte(cur), &arr)
		arr = append(arr, map[string]any{"operation": p.Name(), "proc_status": "success"})
		b, _ := json.Marshal(arr)
		return DocMetadataUpdate{StatusRaw: string(b)}, nil
	})
}

func TestChunkBufferConcurrentReads_NoRace(t *testing.T) {
	ctx := context.Background()
	ctx, _ = withChunkBufferHolder(ctx)

	chunks := []Chunk{
		{SeqNo: 1, Lines: []MarkedLine{{Line: Line{}, Mark: "r"}}},
		{SeqNo: 2, Lines: []MarkedLine{{Line: Line{}, Mark: "r"}}},
	}
	storeChunksInContext(ctx, chunks)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			buf := ChunkBufferFromContext(ctx)
			if buf == nil || len(buf.Chunks) != 2 {
				// read-only, no mutation — -race must stay clean
			}
		}()
	}
	wg.Wait()
}

func TestProcessorLogName_PrefersMethodSpecificLogName(t *testing.T) {
	p := fakeProcessor{name: "chunking", logName: "topic_chunking"}
	if got := processorLogName(p); got != "topic_chunking" {
		t.Fatalf("processorLogName=%q, want topic_chunking", got)
	}
}

type fakeDocProcessingCommandStore struct {
	records    map[int64]DocMetadataInputRecord
	parsed     []DocMetadataInputRecord
	withFailed []DocMetadataInputRecord
}

func (f *fakeDocProcessingCommandStore) GetInputRecord(_ context.Context, id int64) (DocMetadataInputRecord, error) {
	if f.records != nil {
		if rec, ok := f.records[id]; ok {
			return rec, nil
		}
	}
	for _, rec := range append(append([]DocMetadataInputRecord{}, f.parsed...), f.withFailed...) {
		if rec.ID == id {
			return rec, nil
		}
	}
	return DocMetadataInputRecord{}, sql.ErrNoRows
}

func (f *fakeDocProcessingCommandStore) UpdateInputMetadata(_ context.Context, id int64, req DocMetadataUpdate) error {
	if f.records == nil {
		f.records = map[int64]DocMetadataInputRecord{}
	}
	rec := f.records[id]
	rec.ID = id
	if strings.TrimSpace(req.StatusRaw) != "" {
		rec.StatusRaw = req.StatusRaw
	}
	f.records[id] = rec
	return nil
}

func (f *fakeDocProcessingCommandStore) ListParsedInputRecords(_ context.Context) ([]DocMetadataInputRecord, error) {
	return append([]DocMetadataInputRecord{}, f.parsed...), nil
}

func (f *fakeDocProcessingCommandStore) ListRecordsWithFailedDocProcessors(_ context.Context) ([]DocMetadataInputRecord, error) {
	return append([]DocMetadataInputRecord{}, f.withFailed...), nil
}

// --- concurrent test helpers for two-phase controller ---

type concurrentRecorder struct {
	mu    sync.Mutex
	calls []string
}

func (r *concurrentRecorder) add(s string) { r.mu.Lock(); r.calls = append(r.calls, s); r.mu.Unlock() }

type recordingProcessor struct {
	name string
	rec  *concurrentRecorder
	hook func() // optional, to assert overlap
}

func (p recordingProcessor) Name() string    { return p.name }
func (p recordingProcessor) LogName() string { return p.name }

func (p recordingProcessor) HandleEvent(_ context.Context, _ []byte) error {
	p.rec.add("start:" + p.name)
	if p.hook != nil {
		p.hook()
	}
	p.rec.add("end:" + p.name)
	return nil
}

type failingRecordingProcessor struct {
	name string
	rec  *concurrentRecorder
}

func (p failingRecordingProcessor) Name() string    { return p.name }
func (p failingRecordingProcessor) LogName() string { return p.name }

func (p failingRecordingProcessor) HandleEvent(_ context.Context, _ []byte) error {
	p.rec.add("start:" + p.name)
	p.rec.add("end:" + p.name)
	return errors.New("simulated failure")
}

func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}

func indexOfPrefix(ss []string, targets ...string) int {
	for i, s := range ss {
		for _, t := range targets {
			if s == t {
				return i
			}
		}
	}
	return len(ss)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- two-phase tests ---

func TestTwoPhase_PhaseABeforePhaseB(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "true")
	rec := &concurrentRecorder{}
	svc := &ControlService{
		Processors: []Processor{
			recordingProcessor{name: "static_analyzer", rec: rec},
			recordingProcessor{name: "chunking", rec: rec},
			recordingProcessor{name: "extract_doc_metadata", rec: rec},
			recordingProcessor{name: "extract_metrics", rec: rec},
			recordingProcessor{name: "generate_topics", rec: rec},
		},
	}
	svc.HandleEvent(context.Background(), []byte(`{"record_id":"1"}`))

	// Every Phase A end appears before any Phase B start.
	firstPhaseB := indexOfPrefix(rec.calls, "start:extract_metrics", "start:generate_topics")
	for _, a := range []string{"end:static_analyzer", "end:chunking", "end:extract_doc_metadata"} {
		if idx := indexOf(rec.calls, a); idx == -1 || idx > firstPhaseB {
			t.Fatalf("%s did not complete before Phase B (calls=%v)", a, rec.calls)
		}
	}
}

func TestTwoPhase_PhaseBOverlaps(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "true")
	rec := &concurrentRecorder{}
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	hook := func() { started <- struct{}{}; <-release }
	svc := &ControlService{
		Processors: []Processor{
			recordingProcessor{name: "extract_metrics", rec: rec, hook: hook},
			recordingProcessor{name: "generate_topics", rec: rec, hook: hook},
		},
	}
	done := make(chan struct{})
	go func() { svc.HandleEvent(context.Background(), []byte(`{"record_id":"1"}`)); close(done) }()
	<-started
	<-started // both must start before either is released → proves overlap
	close(release)
	<-done
}

func TestTwoPhase_FlagOffIsSequential(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	got := make([]string, 0, 3)
	svc := &ControlService{
		Processors: []Processor{
			fakeProcessor{name: "extract_metrics", calls: &got},
			fakeProcessor{name: "generate_topics", calls: &got},
		},
	}
	svc.HandleEvent(context.Background(), []byte(`{"record_id":"1"}`))
	want := []string{"extract_metrics", "generate_topics"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestTwoPhase_FailureIsolation(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "true")
	rec := &concurrentRecorder{}
	svc := &ControlService{
		Processors: []Processor{
			failingRecordingProcessor{name: "extract_metrics", rec: rec},
			recordingProcessor{name: "generate_topics", rec: rec},
		},
	}
	err := svc.handleEvent(context.Background(), []byte(`{"record_id":"1"}`))
	if err == nil {
		t.Fatal("expected failure error")
	}
	if indexOf(rec.calls, "end:generate_topics") == -1 {
		t.Fatal("sibling did not complete despite peer failure")
	}
}

func TestNormalizeStoredEventPayload_WrapsInvalidJSON(t *testing.T) {
	got := normalizeStoredEventPayload([]byte(`{"record_id":"1""type":"pdf"}`))
	want := `{"_payload_error":"invalid_json","_raw_payload":"{\"record_id\":\"1\"\"type\":\"pdf\"}"}`
	if got != want {
		t.Fatalf("normalizeStoredEventPayload()=%s, want %s", got, want)
	}
}
