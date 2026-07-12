package docprocessing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type fakeRunStoreClose struct {
	runID  int64
	status string
	errMsg *string
}

// fakeRunStore is a test double for DocProcessRunStore that records every
// create/close call so handleEvent's run lifecycle wiring can be asserted
// without a real database. See ADR 2026071201.
type fakeRunStore struct {
	mu        sync.Mutex
	nextID    int64
	creates   []DocProcessRunRecord
	closes    []fakeRunStoreClose
	createErr error
}

func (f *fakeRunStore) CreateDocProcessRun(_ context.Context, rec DocProcessRunRecord) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return 0, f.createErr
	}
	f.nextID++
	f.creates = append(f.creates, rec)
	return f.nextID, nil
}

func (f *fakeRunStore) CloseDocProcessRun(_ context.Context, runID int64, status string, errMsg *string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes = append(f.closes, fakeRunStoreClose{runID: runID, status: status, errMsg: errMsg})
	return nil
}

// runIDCapturingProcessor records the run id (if any) visible on its ctx when
// handleEvent invokes it, so tests can assert the id created for this
// invocation actually reaches processor call sites.
type runIDCapturingProcessor struct {
	name string
	got  *int64
	ok   *bool
}

func (p runIDCapturingProcessor) Name() string { return p.name }

func (p runIDCapturingProcessor) HandleEvent(ctx context.Context, _ []byte) error {
	id, ok := runIDFromContext(ctx)
	*p.got = id
	*p.ok = ok
	return nil
}

func TestHandleEvent_CreatesAndClosesRunAndThreadsRunIDToProcessors(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:              4821,
			ParserName:      "opendata",
			ResultFilename:  "result.json",
			StagingFilename: inputPath,
			StatusRaw:       "[]",
		},
	}
	runStore := &fakeRunStore{}
	var gotRunID int64
	var gotOK bool
	svc := &ControlService{
		InputStore: store,
		RunStore:   runStore,
		Processors: []Processor{
			runIDCapturingProcessor{name: "extract_metrics", got: &gotRunID, ok: &gotOK},
		},
	}

	err := svc.handleEvent(context.Background(), []byte(`{
		"record_id":"4821",
		"filename":"`+inputPath+`",
		"operation":["extract_metrics"]
	}`))
	if err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	if !gotOK {
		t.Fatal("expected extract_metrics to see a run id on its context")
	}

	runStore.mu.Lock()
	defer runStore.mu.Unlock()
	if len(runStore.creates) != 1 {
		t.Fatalf("creates=%d, want 1", len(runStore.creates))
	}
	create := runStore.creates[0]
	if create.RecordID != 4821 {
		t.Errorf("RecordID=%d, want 4821", create.RecordID)
	}
	if create.Mode != "auto" {
		t.Errorf("Mode=%q, want auto", create.Mode)
	}
	if len(create.Processors) != 1 || create.Processors[0] != "extract_metrics" {
		t.Errorf("Processors=%v, want [extract_metrics]", create.Processors)
	}
	if force, ok := create.Parameters["force"].(bool); !ok || !force {
		t.Errorf("Parameters[force]=%v, want true (event omitted force, defaults to true)", create.Parameters["force"])
	}

	if len(runStore.closes) != 1 {
		t.Fatalf("closes=%d, want 1", len(runStore.closes))
	}
	closeCall := runStore.closes[0]
	if closeCall.runID != gotRunID {
		t.Errorf("close runID=%d, want %d (the id created for this invocation)", closeCall.runID, gotRunID)
	}
	if closeCall.status != "success" {
		t.Errorf("close status=%q, want success", closeCall.status)
	}
}

func TestHandleEvent_ClosesRunAsFailedWhenProcessorFails(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	store := &fakeDocMetadataStore{
		rec: DocMetadataInputRecord{
			ID:              4821,
			ParserName:      "opendata",
			ResultFilename:  "result.json",
			StagingFilename: inputPath,
			StatusRaw:       "[]",
		},
	}
	runStore := &fakeRunStore{}
	svc := &ControlService{
		InputStore: store,
		RunStore:   runStore,
		Processors: []Processor{
			fakeProcessor{name: "extract_metrics", retErr: errors.New("boom")},
		},
	}

	_ = svc.handleEvent(context.Background(), []byte(`{
		"record_id":"4821",
		"filename":"`+inputPath+`",
		"operation":["extract_metrics"]
	}`))

	runStore.mu.Lock()
	defer runStore.mu.Unlock()
	if len(runStore.closes) != 1 {
		t.Fatalf("closes=%d, want 1", len(runStore.closes))
	}
	if runStore.closes[0].status != "failed" {
		t.Errorf("close status=%q, want failed", runStore.closes[0].status)
	}
}

func TestHandleEvent_SkippedEventCreatesNoRun(t *testing.T) {
	runStore := &fakeRunStore{}
	svc := &ControlService{
		RunStore: runStore,
	}

	// Empty payload / missing record_id triggers an early parse failure,
	// which returns before any run is created.
	_ = svc.handleEvent(context.Background(), []byte(`{}`))

	runStore.mu.Lock()
	defer runStore.mu.Unlock()
	if len(runStore.creates) != 0 {
		t.Fatalf("creates=%d, want 0 for a skipped/failed-to-parse event", len(runStore.creates))
	}
}
