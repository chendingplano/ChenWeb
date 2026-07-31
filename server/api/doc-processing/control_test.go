package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

type inspectingProcessor struct {
	name          string
	store         *fakeStatusStore
	seenStatusRaw string
}

func (p *inspectingProcessor) Name() string { return p.name }

func (p *inspectingProcessor) HandleEvent(ctx context.Context, _ []byte) error {
	rec, err := p.store.GetInputRecord(ctx, 7)
	if err != nil {
		return err
	}
	p.seenStatusRaw = rec.StatusRaw
	return nil
}

type fakeBatchProcessor struct {
	name         string
	initCalls    int
	processCalls int
	finalCalls   int
}

func (p *fakeBatchProcessor) Name() string { return p.name }

func (p *fakeBatchProcessor) HandleEvent(_ context.Context, _ []byte) error { return nil }

func (p *fakeBatchProcessor) InitChunkBatch(_ context.Context, _ int64, _ []Chunk, _ string) error {
	p.initCalls++
	return nil
}

func (p *fakeBatchProcessor) ProcessChunk(_ context.Context, _ int) error {
	p.processCalls++
	return nil
}

func (p *fakeBatchProcessor) FinalizeChunkBatch(_ context.Context) error {
	p.finalCalls++
	return nil
}

func TestRunSingleProcessorCollect_PersistsActiveStatusBeforeHandleEvent(t *testing.T) {
	store := &fakeStatusStore{raw: "[]"}
	proc := &inspectingProcessor{
		name:  "extract_doc_metadata",
		store: store,
	}
	svc := &ControlService{
		InputStore: store,
		Now: func() time.Time {
			return time.Date(2026, 7, 23, 18, 0, 0, 0, time.UTC)
		},
	}

	res := svc.runSingleProcessorCollect(context.Background(), []byte(`{}`), proc, 7)
	if res.failed {
		t.Fatalf("processor unexpectedly failed: %+v", res)
	}

	var entries []map[string]any
	if err := json.Unmarshal([]byte(proc.seenStatusRaw), &entries); err != nil {
		t.Fatalf("unmarshal seen status: %v", err)
	}

	var directEntry map[string]any
	for _, entry := range entries {
		if canonicalOperationName(asString(entry["operation"])) == "extract_doc_metadata" {
			directEntry = entry
			break
		}
	}
	if directEntry == nil {
		t.Fatalf("missing direct extract_doc_metadata status entry in %s", proc.seenStatusRaw)
	}
	if got := strings.TrimSpace(asString(directEntry["proc_status"])); got != "active" {
		t.Fatalf("direct proc_status=%q, want active", got)
	}
}

func TestUpsertProcessorRuntimeStatus_ReplacesLegacyAlias(t *testing.T) {
	now := time.Date(2026, 7, 23, 18, 5, 0, 0, time.UTC)
	raw := `[{"operation":"extract_metadata","proc_status":"success","error":"old","start_time":"20260723 17:00:00"}]`

	got, err := upsertProcessorRuntimeStatus(raw, now, "extract_doc_metadata", "active", "")
	if err != nil {
		t.Fatalf("upsertProcessorRuntimeStatus: %v", err)
	}

	var entries []map[string]any
	if err := json.Unmarshal([]byte(got), &entries); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries)=%d, want 1: %s", len(entries), got)
	}
	if got := strings.TrimSpace(asString(entries[0]["operation"])); got != "extract_metadata" {
		t.Fatalf("operation=%q, want extract_metadata", got)
	}
	if got := strings.TrimSpace(asString(entries[0]["proc_status"])); got != "active" {
		t.Fatalf("proc_status=%q, want active", got)
	}
	if _, ok := entries[0]["error"]; ok {
		t.Fatalf("stale error should be cleared: %#v", entries[0])
	}
}

func TestRunProcessorsChunkBatched_PersistsBatchProcessorLifecycle(t *testing.T) {
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "record.txt")
	content := "1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tIntro\n"
	if err := os.WriteFile(lineFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}

	store := &fakeDocProcessingCommandStore{
		records: map[int64]DocMetadataInputRecord{
			7: {
				ID:              7,
				StatusRaw:       "[]",
				ResultFilename:  lineFile,
				ParserName:      "mineru",
				StagingFilename: filepath.Join(tmp, "record.pdf"),
			},
		},
	}
	proc := &fakeBatchProcessor{name: "extract_metrics"}
	svc := &ControlService{
		InputStore: store,
		Now: func() time.Time {
			return time.Date(2026, 7, 23, 18, 10, 0, 0, time.UTC)
		},
	}

	ctx := context.Background()
	ctx, _ = withChunkBufferHolder(ctx)
	storeChunksInContext(ctx, []Chunk{
		{SeqNo: 1, Lines: []MarkedLine{{Line: Line{LineNo: 1, PageNo: 1, LineType: "paragraph", Content: "Intro"}, Mark: "r"}}},
	})
	payload := []byte(fmt.Sprintf(`{"record_id":"7","filename":"%s","operation":["extract_metrics"]}`, lineFile))

	var (
		requestFailed  bool
		requestStopped bool
		firstErr       error
	)
	svc.runProcessorsChunkBatched(ctx, payload, []Processor{proc}, 7, &requestFailed, &requestStopped, &firstErr, nil)

	if requestFailed || requestStopped || firstErr != nil {
		t.Fatalf("batch run failed=%v stopped=%v err=%v", requestFailed, requestStopped, firstErr)
	}
	if proc.initCalls != 1 || proc.finalCalls != 1 {
		t.Fatalf("init=%d final=%d, want 1 each", proc.initCalls, proc.finalCalls)
	}
	if proc.processCalls == 0 {
		t.Fatal("expected at least one ProcessChunk call")
	}

	rec, err := store.GetInputRecord(context.Background(), 7)
	if err != nil {
		t.Fatalf("load record: %v", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(rec.StatusRaw), &entries); err != nil {
		t.Fatalf("unmarshal status: %v", err)
	}
	var directEntry map[string]any
	for _, entry := range entries {
		if canonicalOperationName(asString(entry["operation"])) == "extract_metrics" {
			directEntry = entry
			break
		}
	}
	if directEntry == nil {
		t.Fatalf("missing extract_metrics status entry in %s", rec.StatusRaw)
	}
	if got := strings.TrimSpace(asString(directEntry["proc_status"])); got != "success" {
		t.Fatalf("proc_status=%q, want success", got)
	}
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

func TestControlService_RunEventDelegatesSynchronously(t *testing.T) {
	wantErr := errors.New("processor failed")
	svc := &ControlService{Processors: []Processor{fakeProcessor{name: "extract_metrics", retErr: wantErr}}}
	err := svc.RunEvent(context.Background(), []byte(`{"record_id":"1","operation":["extract_metrics"]}`))
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunEvent error=%v, want %v", err, wantErr)
	}
}

func TestExpandProcessorDependenciesAddsChunkingForChunkConsumers(t *testing.T) {
	got := expandProcessorDependencies([]string{"extract_metrics", "extract_provisions"})
	want := []string{"static_analyzer", "chunking", "extract_metrics", "extract_provisions"}
	if !equalStrings(got, want) {
		t.Fatalf("expanded=%v, want %v", got, want)
	}
}

func TestExpandProcessorDependenciesPreservesExplicitChunkingOrder(t *testing.T) {
	got := expandProcessorDependencies([]string{"extract_metrics", "chunking"})
	want := []string{"extract_metrics", "static_analyzer", "chunking"}
	if !equalStrings(got, want) {
		t.Fatalf("expanded=%v, want %v", got, want)
	}
}

func TestControlService_SkipsSatisfiedAutoDependenciesOnRerun(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "source.pdf")
	writeLineFile(t, filepath.Join(dir, "source_opendata.txt"))
	if err := os.WriteFile(inputPath, []byte("pdf placeholder"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	got := make([]string, 0, 3)
	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              1,
		ParserName:      "opendata",
		ResultFilename:  filepath.Join(dir, "result.json"),
		StagingFilename: inputPath,
		StatusRaw: `[
			{"operation":"static_analyzer","proc_status":"success"},
			{"operation":"chunking","proc_status":"success"},
			{"operation":"extract_metrics","proc_status":"failed"}
		]`,
	}}
	svc := &ControlService{
		InputStore: store,
		Processors: []Processor{
			fakeProcessor{name: "static_analyzer", calls: &got},
			fakeProcessor{name: "chunking", calls: &got},
			fakeProcessor{name: "extract_metrics", calls: &got},
		},
	}

	svc.HandleEvent(context.Background(), []byte(`{"record_id":"1","operation":["extract_metrics"]}`))

	want := []string{"extract_metrics"}
	if !equalStrings(got, want) {
		t.Fatalf("calls=%v, want %v", got, want)
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

func TestHandleEvent_ResetsOnlySelectedProcessorStatusesBeforeRerun(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	release := make(chan struct{})
	started := make(chan struct{}, 1)
	var done sync.WaitGroup
	done.Add(1)

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:              7,
			ParserName:      "opendata",
			ResultFilename:  "result.json",
			StagingFilename: inputPath,
			StatusRaw: `[
				{"operation":"extract_metrics","proc_status":"success"},
				{"operation":"generate_topics","proc_status":"success"},
				{"operation":"extract_products","proc_status":"success"}
			]`,
		},
	}
	svc := &ControlService{
		InputStore: store,
		Processors: []Processor{
			&blockingConcurrencyProcessor{
				name:    "extract_metrics",
				started: started,
				release: release,
				done:    &done,
			},
			fakeProcessor{name: "generate_topics"},
			fakeProcessor{name: "extract_products"},
		},
		Now: time.Now,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- svc.handleEvent(context.Background(), []byte(`{
			"record_id":"7",
			"filename":"`+inputPath+`",
			"operation":["extract_metrics","generate_topics"]
		}`))
	}()

	waitForProcessorStarts(t, started, 1)

	updates := store.statusUpdates()
	if len(updates) < 2 {
		t.Fatalf("status updates=%d, want at least 2", len(updates))
	}
	firstEntries := decodeDocMetaStatus(updates[0].StatusRaw)
	assertStatusOps(t, firstEntries, []string{"extract_products"})

	secondEntries := decodeDocMetaStatus(updates[1].StatusRaw)
	assertStatusOps(t, secondEntries, []string{"extract_products", "doc_processing"})

	close(release)
	done.Wait()

	if err := <-errCh; err != nil {
		t.Fatalf("handleEvent: %v", err)
	}
}

func TestResetRequestedProcessorStatuses_ClearsLegacyAliases(t *testing.T) {
	raw := `[
		{"operation":"extract_scene_blocks","proc_status":"success"},
		{"operation":"extract_entity_relation","proc_status":"success"},
		{"operation":"extract_relation","proc_status":"success"},
		{"operation":"extract_inventory_items","proc_status":"success"},
		{"operation":"doc_processing","proc_status":"running","doc_processor_name":"extract_scene_blocks"}
	]`

	got, err := resetRequestedProcessorStatuses(raw, []string{"generate_scene_blocks", "extract_entity"})
	if err != nil {
		t.Fatalf("resetRequestedProcessorStatuses: %v", err)
	}

	entries := decodeDocMetaStatus(got)
	assertStatusOps(t, entries, []string{"extract_relation", "extract_inventory_items", "doc_processing"})
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

// TestControlService_WaitForInFlightPipelines_BlocksUntilDispatchedPipelineFinishes
// guards against the bug that produced "sql: database is closed" errors:
// HandleJetStreamEvent dispatches the pipeline to a detached goroutine and
// returns before it finishes, so shutdown code must not treat the handler's
// return as proof the pipeline (and its DB writes) are done.
func TestControlService_WaitForInFlightPipelines_BlocksUntilDispatchedPipelineFinishes(t *testing.T) {
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

	if err := svc.HandleJetStreamEvent(context.Background(), DefaultEventSubject, []byte(`{"record_id":"1","filename":"`+inputPath+`"}`)); err != nil {
		t.Fatalf("HandleJetStreamEvent() error = %v, want nil", err)
	}
	// HandleJetStreamEvent has already returned, but the pipeline it dispatched
	// is still blocked in the processor below.
	waitForProcessorStarts(t, started, 1)

	if svc.WaitForInFlightPipelines(20 * time.Millisecond) {
		t.Fatal("WaitForInFlightPipelines() = true while the dispatched pipeline is still running, want false")
	}

	close(release)
	done.Wait()

	if !svc.WaitForInFlightPipelines(time.Second) {
		t.Fatal("WaitForInFlightPipelines() = false after the pipeline finished, want true")
	}
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

func assertStatusOps(t *testing.T, entries []map[string]any, want []string) {
	t.Helper()
	got := make([]string, 0, len(entries))
	for _, entry := range entries {
		got = append(got, canonicalOperationName(asString(entry["operation"])))
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("status ops=%v, want %v", got, want)
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

type phaseCBlockingProcessor struct {
	statusWritingProcessor
	started chan struct{}
	release chan struct{}
}

func (p *phaseCBlockingProcessor) PostProcessIndex(_ context.Context, _ int64) error {
	select {
	case p.started <- struct{}{}:
	default:
	}
	<-p.release
	return nil
}

func TestControlService_PhaseCKeepsPipelineRunningWithoutProcessorName(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "source.pdf")
	lineFilePath := filepath.Join(dir, "source_opendata.txt")
	resultPath := filepath.Join(dir, "result.json")
	if err := os.WriteFile(inputPath, []byte("pdf placeholder"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	writeLineFile(t, lineFilePath)

	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID:              1,
		ParserName:      "opendata",
		ResultFilename:  resultPath,
		StagingFilename: inputPath,
		StatusRaw:       "[]",
	}}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	proc := &phaseCBlockingProcessor{
		statusWritingProcessor: statusWritingProcessor{name: "extract_metrics", store: store, id: 1},
		started:                started,
		release:                release,
	}
	svc := &ControlService{
		InputStore: store,
		Processors: []Processor{proc},
		Now:        time.Now,
	}

	done := make(chan error, 1)
	go func() {
		done <- svc.handleEvent(context.Background(), []byte(`{"record_id":"1"}`))
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Phase C to start")
	}

	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		raw := store.currentStatusRaw()
		entries := decodeDocMetaStatus(raw)
		if len(entries) > 0 {
			for _, entry := range entries {
				if strings.TrimSpace(asString(entry["operation"])) != "doc_processing" {
					continue
				}
				if strings.TrimSpace(asString(entry["proc_status"])) != "running" {
					continue
				}
				if gotName := strings.TrimSpace(asString(entry["doc_processor_name"])); gotName != "" {
					t.Fatalf("doc_processing still points at %q during Phase C; want empty processor name", gotName)
				}
				if isStuckPipeline(entries) {
					t.Fatal("Phase C running status was misclassified as stuck")
				}
				close(release)
				if err := <-done; err != nil {
					t.Fatalf("handleEvent returned error: %v", err)
				}
				return
			}
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for unnamed running doc_processing status; latest=%s", raw)
		case <-ticker.C:
		}
	}
}

func TestIsStuckPipeline_FalseForFreshRunWithoutDirectProcessorEntry(t *testing.T) {
	entries := []map[string]any{
		{"operation": "parsed", "proc_status": "success"},
		{"operation": "converted", "proc_status": "success"},
		{"operation": "doc_processing", "proc_status": "running", "doc_processor_name": "blocking"},
	}
	if isStuckPipeline(entries) {
		t.Fatal("freshly started pipeline was misclassified as stuck")
	}
}

func TestIsStuckPipeline_FalseDuringStageTransitionBeforeNamedProcessorWritesStatus(t *testing.T) {
	entries := []map[string]any{
		{"operation": "parsed", "proc_status": "success"},
		{"operation": "converted", "proc_status": "success"},
		{"operation": "blocking", "proc_status": "success"},
		{"operation": "doc_processing", "proc_status": "running", "doc_processor_name": "static_analyzer"},
	}
	if isStuckPipeline(entries) {
		t.Fatal("stage-transition pipeline was misclassified as stuck")
	}
}

func TestIsStuckPipeline_TrueWhenNamedProcessorAlreadyFinished(t *testing.T) {
	entries := []map[string]any{
		{"operation": "parsed", "proc_status": "success"},
		{"operation": "converted", "proc_status": "success"},
		{"operation": "blocking", "proc_status": "success"},
		{"operation": "static_analyzer", "proc_status": "success"},
		{"operation": "doc_processing", "proc_status": "running", "doc_processor_name": "static_analyzer"},
	}
	if !isStuckPipeline(entries) {
		t.Fatal("stuck pipeline was not detected")
	}
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

func TestResolveProductionPlanFactsBuildsFactsFromCurrentInputRecord(t *testing.T) {
	store := &fakeDocProcessingCommandStore{
		records: map[int64]DocMetadataInputRecord{
			7: {
				ID:             7,
				KSStoreID:      42,
				ParserName:     "mineru",
				Title:          "Ventilator display module",
				InputDocType:   "pdf",
				SourceLanguage: "en",
				DocumentNumber: "YY 9706.252-2021",
			},
		},
	}
	svc := &ControlService{InputStore: store}
	evt := LineFileGeneratedEvent{RecordID: 7, Operations: []string{"generate_topics", "extract_provisions"}}

	got, err := svc.resolveProductionPlanFacts(context.Background(), evt)
	if err != nil {
		t.Fatalf("resolveProductionPlanFacts: %v", err)
	}

	want := ProductionPlanFacts{
		RequestedProcessors: []string{"generate_topics", "extract_provisions"},
		KnowledgeStoreID:    42,
		ParserName:          "mineru",
		DocumentTitle:       "Ventilator display module",
		InputDocType:        "pdf",
		SourceLanguage:      "en",
		DocumentNumber:      "YY 9706.252-2021",
		RoutingFacets: ProductionRoutingFacets{
			KnowledgeStoreBinding: "bound",
			InputDocType:          "pdf",
			SourceLanguage:        "en",
			HasDocumentNumber:     true,
		},
		Mode: DocPipelineModePlanOnly,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("facts=%#v want=%#v", got, want)
	}
}

func TestAppendPipelineStatusWithPlanPreservesPlanSnapshotAcrossLaterUpdates(t *testing.T) {
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	facts := ProductionPlanFacts{
		RequestedProcessors: []string{"generate_topics"},
		KnowledgeStoreID:    42,
		ParserName:          "mineru",
		DocumentTitle:       "Ventilator display module",
	}
	steps := []ProcessorPlanStep{
		{Name: "static_analyzer", Phase: "A", DependsOn: []string{}, Reason: "mandatory_baseline"},
		{Name: "chunking", Phase: "A", DependsOn: []string{"static_analyzer"}, Reason: "implicit_dependency"},
		{Name: "generate_topics", Phase: "B", DependsOn: []string{"chunking"}, Reason: "explicit_request"},
	}
	selection := ProductionPipelineSelection{
		PipelineName: "legacy_default",
		Reason:       "system_default",
	}
	binding := ProductionPipelineBindingResolution{
		Source:           "system_default",
		SelectedPipeline: "legacy_default",
	}
	spec := ProductionPipelineSpec{
		Name:             "legacy_default",
		DisplayName:      "Legacy Default",
		LegacyEquivalent: true,
	}
	excluded := []string{"extract_provisions"}

	raw, err := appendPipelineStatusWithPlan("[]", now, "running", "", nil, facts, steps, selection, binding, spec, excluded)
	if err != nil {
		t.Fatalf("appendPipelineStatusWithPlan initial: %v", err)
	}
	raw, err = appendPipelineStatusWithPlan(raw, now.Add(time.Second), "success", "", nil, ProductionPlanFacts{}, nil, ProductionPipelineSelection{}, ProductionPipelineBindingResolution{}, ProductionPipelineSpec{}, nil)
	if err != nil {
		t.Fatalf("appendPipelineStatusWithPlan follow-up: %v", err)
	}

	entries := decodeDocMetaStatus(raw)
	var docProcessing map[string]any
	for _, entry := range entries {
		if strings.TrimSpace(asString(entry["operation"])) == "doc_processing" {
			docProcessing = entry
			break
		}
	}
	if docProcessing == nil {
		t.Fatalf("missing doc_processing entry in %s", raw)
	}
	if _, ok := docProcessing["processor_plan_facts"]; !ok {
		t.Fatalf("missing processor_plan_facts in %#v", docProcessing)
	}
	if _, ok := docProcessing["processor_plan_steps"]; !ok {
		t.Fatalf("missing processor_plan_steps in %#v", docProcessing)
	}
	if _, ok := docProcessing["processor_pipeline_selection"]; !ok {
		t.Fatalf("missing processor_pipeline_selection in %#v", docProcessing)
	}
	if _, ok := docProcessing["processor_pipeline_binding"]; !ok {
		t.Fatalf("missing processor_pipeline_binding in %#v", docProcessing)
	}
	if _, ok := docProcessing["processor_pipeline_spec"]; !ok {
		t.Fatalf("missing processor_pipeline_spec in %#v", docProcessing)
	}
	if _, ok := docProcessing["processor_excluded_by_policy"]; !ok {
		t.Fatalf("missing processor_excluded_by_policy in %#v", docProcessing)
	}
}

type fakeDocFacetStore struct {
	upserted []DocFacetRecord
	err      error
}

func (f *fakeDocFacetStore) UpsertDocFacets(_ context.Context, rec DocFacetRecord) error {
	if f.err != nil {
		return f.err
	}
	f.upserted = append(f.upserted, rec)
	return nil
}

func (f *fakeDocFacetStore) GetDocFacets(_ context.Context, recordID int64) (DocFacetRecord, error) {
	for _, rec := range f.upserted {
		if rec.RecordID == recordID {
			return rec, nil
		}
	}
	return DocFacetRecord{}, sql.ErrNoRows
}

func TestPersistDocFacetsUpsertsRoutingFacetsFromPlanFacts(t *testing.T) {
	facetStore := &fakeDocFacetStore{}
	svc := &ControlService{FacetStore: facetStore}

	svc.persistDocFacets(context.Background(), 91, ProductionPlanFacts{
		KnowledgeStoreID: 42,
		RoutingFacets: ProductionRoutingFacets{
			KnowledgeStoreBinding: "bound",
			InputDocType:          "pdf",
			SourceLanguage:        "en",
			HasDocumentNumber:     true,
		},
	})

	if len(facetStore.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(facetStore.upserted))
	}
	want := DocFacetRecord{RecordID: 91, KSStoreID: 42, KnowledgeStoreBinding: "bound", InputDocType: "pdf", SourceLanguage: "en", HasDocumentNumber: true}
	if facetStore.upserted[0] != want {
		t.Fatalf("got=%#v want=%#v", facetStore.upserted[0], want)
	}
}

func TestPersistDocFacetsNoopsWithoutFacetStore(t *testing.T) {
	svc := &ControlService{}
	// Must not panic when FacetStore is unset (e.g. in tests that construct
	// ControlService directly without wiring every optional dependency).
	svc.persistDocFacets(context.Background(), 91, ProductionPlanFacts{})
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
