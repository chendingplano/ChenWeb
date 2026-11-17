package docprocessing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// llmRunRecorder collects the llm_run_id each processor phase sees on its context.
type llmRunRecorder struct {
	mu   sync.Mutex
	seen map[string][]int64 // "<processor>/<phase>" -> IDs, one per call
}

func (r *llmRunRecorder) record(ctx context.Context, key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.seen == nil {
		r.seen = map[string][]int64{}
	}
	r.seen[key] = append(r.seen[key], llmRunIDFromContext(ctx))
}

func (r *llmRunRecorder) only(t *testing.T, key string) int64 {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := r.seen[key]
	if len(ids) == 0 {
		t.Fatalf("%s saw no context", key)
	}
	for _, id := range ids {
		if id != ids[0] {
			t.Fatalf("%s saw several llm_run_ids %v, want one", key, ids)
		}
	}
	return ids[0]
}

// countingLLMRunIDs allocates 500, 501, ... and counts the allocations.
type countingLLMRunIDs struct {
	mu   sync.Mutex
	next int64
}

func (c *countingLLMRunIDs) allocate(context.Context) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.next == 0 {
		c.next = 500
	}
	c.next++
	return c.next - 1
}

type llmRunPhaseProcessor struct {
	name string
	rec  *llmRunRecorder
}

func (p llmRunPhaseProcessor) Name() string { return p.name }

func (p llmRunPhaseProcessor) HandleEvent(ctx context.Context, _ []byte) error {
	p.rec.record(ctx, p.name+"/run")
	return nil
}

type llmRunIndexingProcessor struct{ llmRunPhaseProcessor }

func (p llmRunIndexingProcessor) PostProcessIndex(ctx context.Context, _ int64) error {
	p.rec.record(ctx, p.name+"/index")
	return nil
}

// Each processor invocation of a pipeline run gets its own llm_run_id; its Phase C indexing
// keeps that ID. The pipeline's own ID stays on kb.doc_process_runs.
func TestHandleEvent_GivesEachProcessorItsOwnLLMRunID(t *testing.T) {
	t.Setenv("RUN_DOC_PROCESSOR_CONCURRENT", "false")
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	if err := os.WriteFile(inputPath, []byte("1\t1\tparagraph\tFont\t10\t[0,0,1,1]\ttext\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	store := &fakeDocMetadataStore{rec: DocMetadataInputRecord{
		ID: 4821, ParserName: "opendata", ResultFilename: "result.json", StagingFilename: inputPath,
		StatusRaw: "[]", InputDocType: "pdf", SourceLanguage: "en",
	}}
	rec := &llmRunRecorder{}
	ids := &countingLLMRunIDs{}
	runStore := &fakeRunStore{}
	svc := &ControlService{
		InputStore:   store,
		RunStore:     runStore,
		PlanStore:    &fakePlanStore{},
		NextLLMRunID: ids.allocate,
		Processors: []Processor{
			llmRunIndexingProcessor{llmRunPhaseProcessor{name: "extract_metrics", rec: rec}},
			llmRunPhaseProcessor{name: "extract_provisions", rec: rec},
		},
	}

	err := svc.handleEvent(context.Background(), []byte(`{
		"user_id":"test-user",
		"record_id":"4821",
		"filename":"`+inputPath+`",
		"operation":["extract_metrics","extract_provisions"]
	}`))
	if err != nil {
		t.Fatalf("handleEvent: %v", err)
	}

	metrics := rec.only(t, "extract_metrics/run")
	provisions := rec.only(t, "extract_provisions/run")
	if metrics == 0 || provisions == 0 || metrics == provisions {
		t.Fatalf("extract_metrics llm_run_id=%d, extract_provisions llm_run_id=%d, want two different allocated IDs", metrics, provisions)
	}
	if index := rec.only(t, "extract_metrics/index"); index != metrics {
		t.Fatalf("extract_metrics Phase C llm_run_id=%d, want its Phase B ID %d", index, metrics)
	}
	runStore.mu.Lock()
	defer runStore.mu.Unlock()
	if len(runStore.creates) != 1 {
		t.Fatalf("creates=%d, want 1", len(runStore.creates))
	}
	if pipeline := runStore.creates[0].LLMRunID; pipeline == metrics || pipeline == provisions {
		t.Fatalf("pipeline llm_run_id=%d reused by a processor (%d, %d)", pipeline, metrics, provisions)
	}
}

type llmRunBatchProcessor struct{ llmRunPhaseProcessor }

func (p llmRunBatchProcessor) InitChunkBatch(ctx context.Context, _ int64, _ []Chunk, _ string) error {
	p.rec.record(ctx, p.name+"/init")
	return nil
}

func (p llmRunBatchProcessor) ProcessChunk(ctx context.Context, _ int) error {
	p.rec.record(ctx, p.name+"/chunk")
	return nil
}

func (p llmRunBatchProcessor) FinalizeChunkBatch(ctx context.Context) error {
	p.rec.record(ctx, p.name+"/finalize")
	return nil
}

func TestRunProcessorsChunkBatched_GivesEachBatchProcessorItsOwnLLMRunID(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "0")
	tmp := t.TempDir()
	lineFile := filepath.Join(tmp, "record.txt")
	if err := os.WriteFile(lineFile, []byte("1\t1\tparagraph\tTestFont\t12\t[0,0,1,1]\tIntro\n"), 0o644); err != nil {
		t.Fatalf("write line file: %v", err)
	}
	store := &fakeDocProcessingCommandStore{records: map[int64]DocMetadataInputRecord{
		7: {ID: 7, StatusRaw: "[]", ResultFilename: lineFile, ParserName: "mineru", StagingFilename: filepath.Join(tmp, "record.pdf")},
	}}
	rec := &llmRunRecorder{}
	ids := &countingLLMRunIDs{}
	svc := &ControlService{InputStore: store}

	ctx := withProcessorLLMRuns(withLLMRunID(context.Background(), 99), ids.allocate)
	ctx, _ = withChunkBufferHolder(ctx)
	chunk := func(seq int) Chunk {
		return Chunk{SeqNo: seq, Lines: []MarkedLine{{Line: Line{LineNo: seq, PageNo: 1, LineType: "paragraph", Content: "Intro"}, Mark: "r"}}}
	}
	storeChunksInContext(ctx, []Chunk{chunk(1), chunk(2), chunk(3)})
	payload := []byte(fmt.Sprintf(`{"user_id":"test-user","record_id":"7","filename":"%s","operation":["extract_metrics_simple","extract_provisions"]}`, lineFile))
	procs := []Processor{
		llmRunBatchProcessor{llmRunPhaseProcessor{name: "extract_metrics_simple", rec: rec}},
		llmRunBatchProcessor{llmRunPhaseProcessor{name: "extract_provisions", rec: rec}},
	}

	var failed, stopped bool
	var firstErr error
	svc.runProcessorsChunkBatched(ctx, payload, procs, 7, &failed, &stopped, &firstErr, nil)
	if failed || stopped || firstErr != nil {
		t.Fatalf("batch run failed=%v stopped=%v err=%v", failed, stopped, firstErr)
	}

	got := map[string]int64{}
	for _, name := range []string{"extract_metrics_simple", "extract_provisions"} {
		id := rec.only(t, name+"/init")
		for _, phase := range []string{"chunk", "finalize"} {
			if other := rec.only(t, name+"/"+phase); other != id {
				t.Fatalf("%s %s llm_run_id=%d, want its init ID %d", name, phase, other, id)
			}
		}
		if id == 0 || id == 99 {
			t.Fatalf("%s llm_run_id=%d, want its own allocated ID", name, id)
		}
		got[name] = id
	}
	if got["extract_metrics_simple"] == got["extract_provisions"] {
		t.Fatalf("both batch processors share llm_run_id %d", got["extract_provisions"])
	}
}
