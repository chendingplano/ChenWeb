# DeepSeek Cache Optimization Unification (Doc Processors) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the doc processors run the same three-phase DeepSeek prompt-cache scheduling as the doc reviewers (concurrent seed → stagger → concurrent remainder), with per-unit fallback, and convert/correct the six configured chunk-based processors to the `ChunkBatchProcessor` lifecycle.

**Architecture:** Rewrite the doc-processing per-chunk batching coordinator so the seed processor's chunks run concurrently (not serially), the remainder runs concurrently under a `maxTasks` semaphore, and processors that don't implement `ChunkBatchProcessor` run legacy-concurrent alongside instead of disabling batching (the all-or-nothing gate is removed). Fix the entity/relation split so each processor does only its half. Convert `extract_metrics` and `extract_semantic_projections` (2-pass) to the lifecycle.

**Tech Stack:** Go 1.25, `go test` (with `sqlmock` in existing tests), OpenTelemetry spans, DeepSeek via the shared `llm` client.

## Global Constraints

- Package: `docprocessing` under `ChenWeb/server/api/doc-processing/`.
- The `ChunkBatchProcessor` interface is **unchanged**: `Name()`, `InitChunkBatch(ctx, recordID, chunks, docCtx)`, `ProcessChunk(ctx, chunkIdx)`, `FinalizeChunkBatch(ctx)` — see [chunk_batch.go](../../../server/api/doc-processing/chunk_batch.go).
- `LLM_CALL_STAGGER` semantics and `llmCallStagger()` are **unchanged** (env seconds, default 1s).
- Follow the **existing working batch template**: `InventoryItemsProcessor` (`InitChunkBatch`/`ProcessChunk`/`FinalizeChunkBatch` at [extract-inventory-items.go:1976](../../../server/api/doc-processing/extract-inventory-items.go#L1976)) and `ProvisionsProcessor`. New batch code must mirror that shape (accumulate per chunk into batch-state slice; save in `FinalizeChunkBatch`).
- Every chunk LLM call builds input via `canonicalChunkInputText(chunk.Lines, docCtx)` so the cacheable prefix is byte-identical across processors.
- Error-code prefixes: continue the `(MID_2606....)` convention already used in these files; use a fresh unused number per new error site.
- Commit after each task with `jj` (this repo uses jj): `jj commit -m "<msg>\n\nCo-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"`.
- The full plan must land together before enabling in production: partial completion can leave metrics/semantic on the legacy path (fine) but entity/relation must be fixed (Task 4) before the concurrent coordinator (Tasks 2–3) is exercised on a real run.
- After every task: `cd ChenWeb && go build ./server/api/doc-processing/` and `go vet ./server/api/doc-processing/` must be clean.

---

### Task 1: Scheduling helpers — `maxTasks` env + seed-first ordering

**Files:**
- Modify: `server/api/doc-processing/chunk_batch.go`
- Test: `server/api/doc-processing/chunk_batch_test.go` (create)

**Interfaces:**
- Produces:
  - `func maxDocProcessorTasks(fallback int) int` — reads `MAX_DOC_PROCESSOR_TASKS` (int ≥ 1), else `fallback` (min 1).
  - `var multiPassProcessors map[string]struct{}` — names of 2-pass processors that must not seed.
  - `func orderBatchProcessorsSeedFirst(bp []ChunkBatchProcessor) []ChunkBatchProcessor` — returns a copy with a 1-pass processor first (stable otherwise); if all are multi-pass, returns the input order unchanged.

- [ ] **Step 1: Write the failing test**

Create `server/api/doc-processing/chunk_batch_test.go`:

```go
package docprocessing

import (
	"context"
	"os"
	"testing"
)

// fakeBatchProc is a minimal ChunkBatchProcessor for scheduler tests.
type fakeBatchProc struct{ name string }

func (f *fakeBatchProc) Name() string { return f.name }
func (f *fakeBatchProc) InitChunkBatch(context.Context, int64, []Chunk, string) error { return nil }
func (f *fakeBatchProc) ProcessChunk(context.Context, int) error                      { return nil }
func (f *fakeBatchProc) FinalizeChunkBatch(context.Context) error                     { return nil }

func TestMaxDocProcessorTasks(t *testing.T) {
	os.Unsetenv("MAX_DOC_PROCESSOR_TASKS")
	if got := maxDocProcessorTasks(10); got != 10 {
		t.Fatalf("default: want 10, got %d", got)
	}
	os.Setenv("MAX_DOC_PROCESSOR_TASKS", "3")
	defer os.Unsetenv("MAX_DOC_PROCESSOR_TASKS")
	if got := maxDocProcessorTasks(10); got != 3 {
		t.Fatalf("env: want 3, got %d", got)
	}
}

func TestOrderBatchProcessorsSeedFirst(t *testing.T) {
	metrics := &fakeBatchProc{name: "extract_metrics"} // multi-pass
	inv := &fakeBatchProc{name: "extract_inventory_items"}
	ordered := orderBatchProcessorsSeedFirst([]ChunkBatchProcessor{metrics, inv})
	if ordered[0].Name() != "extract_inventory_items" {
		t.Fatalf("seed must be 1-pass, got %s", ordered[0].Name())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run 'TestMaxDocProcessorTasks|TestOrderBatchProcessorsSeedFirst' -v`
Expected: FAIL — `undefined: maxDocProcessorTasks` / `orderBatchProcessorsSeedFirst`.

- [ ] **Step 3: Write minimal implementation**

Append to `server/api/doc-processing/chunk_batch.go` (add `"strconv"` is already imported; keep imports tidy):

```go
// multiPassProcessors are batch processors that make more than one LLM call
// per chunk. They must not be chosen as the cache seed (Task: seed selection);
// the seed should plant each chunk's prefix with a single clean call.
var multiPassProcessors = map[string]struct{}{
	"extract_metrics":               {},
	"extract_semantic_projections":  {},
}

// maxDocProcessorTasks returns the Phase-3 concurrency cap. Controlled by
// MAX_DOC_PROCESSOR_TASKS (int >= 1); falls back to fallback (min 1). Mirrors
// the reviewers' MAX_DOC_REVIEWER_TASKS.
func maxDocProcessorTasks(fallback int) int {
	if fallback < 1 {
		fallback = 1
	}
	v := strings.TrimSpace(os.Getenv("MAX_DOC_PROCESSOR_TASKS"))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}

// orderBatchProcessorsSeedFirst returns a copy of bp with a single-pass
// processor first so it can seed the DeepSeek cache with one call per chunk.
// Stable for all other positions. If every processor is multi-pass, the input
// order is returned unchanged.
func orderBatchProcessorsSeedFirst(bp []ChunkBatchProcessor) []ChunkBatchProcessor {
	out := make([]ChunkBatchProcessor, 0, len(bp))
	seedIdx := -1
	for i, p := range bp {
		if _, multi := multiPassProcessors[p.Name()]; !multi {
			seedIdx = i
			break
		}
	}
	if seedIdx <= 0 {
		return append(out, bp...) // already seed-first, or all multi-pass
	}
	out = append(out, bp[seedIdx])
	for i, p := range bp {
		if i == seedIdx {
			continue
		}
		out = append(out, p)
	}
	return out
}
```

Confirm `chunk_batch.go` imports include `"os"`, `"strconv"`, `"strings"` (they already do per the current file).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run 'TestMaxDocProcessorTasks|TestOrderBatchProcessorsSeedFirst' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd ChenWeb && jj commit -m "feat(doc-proc): add maxDocProcessorTasks + seed-first ordering helpers

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Coordinator scheduling rewrite — concurrent seed, stagger, bounded remainder

**Files:**
- Modify: `server/api/doc-processing/chunk_batch_coordinator.go` — the Phase 1/2/3 block of `runProcessorsChunkBatched` (currently lines ~178–295).
- Test: `server/api/doc-processing/chunk_batch_coordinator_test.go` (create)

**Interfaces:**
- Consumes: `orderBatchProcessorsSeedFirst`, `maxDocProcessorTasks`, `llmCallStagger` (Task 1 + existing).
- Produces: no new exported symbols; behavior change to `runProcessorsChunkBatched`.

**Context:** Keep everything in `runProcessorsChunkBatched` up to and including the `InitChunkBatch` loop and the `stagger := llmCallStagger()` line. Replace the three-phase body (the seed `for` loop, the `time.After` block, and the Phase-3 goroutine fan-out) with the version below. Keep the `FinalizeChunkBatch` loop and error handling after it unchanged.

- [ ] **Step 1: Write the failing test**

Create `server/api/doc-processing/chunk_batch_coordinator_test.go`:

```go
package docprocessing

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingProc records the wall-clock start of each ProcessChunk call and how
// many run concurrently, so tests can assert seed concurrency and staggering.
type recordingProc struct {
	name       string
	delay      time.Duration
	mu         sync.Mutex
	starts     []time.Time
	concurrent int32
	maxConc    int32
}

func (r *recordingProc) Name() string { return r.name }
func (r *recordingProc) InitChunkBatch(context.Context, int64, []Chunk, string) error { return nil }
func (r *recordingProc) FinalizeChunkBatch(context.Context) error                     { return nil }
func (r *recordingProc) ProcessChunk(ctx context.Context, idx int) error {
	n := atomic.AddInt32(&r.concurrent, 1)
	for {
		old := atomic.LoadInt32(&r.maxConc)
		if n <= old || atomic.CompareAndSwapInt32(&r.maxConc, old, n) {
			break
		}
	}
	r.mu.Lock()
	r.starts = append(r.starts, time.Now())
	r.mu.Unlock()
	time.Sleep(r.delay)
	atomic.AddInt32(&r.concurrent, -1)
	return nil
}

func TestSeedPhaseRunsChunksConcurrently(t *testing.T) {
	t.Setenv("LLM_CALL_STAGGER", "0")
	seed := &recordingProc{name: "extract_inventory_items", delay: 50 * time.Millisecond}
	remainder := &recordingProc{name: "extract_provisions", delay: 10 * time.Millisecond}

	s := &ControlService{}
	chunks := make([]Chunk, 4)
	// Exercise only the scheduling core via the extracted helper (Step 3 extracts it).
	s.scheduleChunkBatch(context.Background(),
		[]ChunkBatchProcessor{seed, remainder}, chunks, 1)

	if got := atomic.LoadInt32(&seed.maxConc); got < 2 {
		t.Fatalf("seed chunks should overlap; max concurrency was %d, want >= 2", got)
	}
	if len(seed.starts) != 4 || len(remainder.starts) != 4 {
		t.Fatalf("each processor should run all 4 chunks; seed=%d remainder=%d",
			len(seed.starts), len(remainder.starts))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run TestSeedPhaseRunsChunksConcurrently -v`
Expected: FAIL — `s.scheduleChunkBatch undefined`.

- [ ] **Step 3: Extract the scheduling core and rewrite it**

In `chunk_batch_coordinator.go`, add a new method that contains the three-phase schedule, and call it from `runProcessorsChunkBatched` (replacing the old Phase 1/2/3 body). Returns the first `ProcessChunk` error seen (or nil). It does **not** call `Init`/`Finalize` — the caller still owns those.

```go
// scheduleChunkBatch runs the three-phase DeepSeek cache schedule over already
// initialised batch processors: (1) fire the seed processor's chunks
// concurrently, (2) wait LLM_CALL_STAGGER for the prefixes to persist, (3) fire
// all remaining (processor x chunk) calls concurrently, bounded by
// MAX_DOC_PROCESSOR_TASKS. Mirrors the doc-reviewers' runReviewTasksForPromptCache.
// Returns the first ProcessChunk error, or ErrPipelineStopped on cancel.
func (s *ControlService) scheduleChunkBatch(
	ctx context.Context,
	batchProcessors []ChunkBatchProcessor,
	chunks []Chunk,
	recordID int64,
) error {
	if len(batchProcessors) == 0 || len(chunks) == 0 {
		return nil
	}
	ordered := orderBatchProcessorsSeedFirst(batchProcessors)
	seed := ordered[0]

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		firstErr error
	)
	record := func(err error) {
		if err == nil {
			return
		}
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}
	runOne := func(bp ChunkBatchProcessor, chunkIdx int) {
		if isCtxStopped(ctx) {
			record(ErrPipelineStopped)
			return
		}
		recCtx := withLLMRecordID(ctx, recordID)
		if err := bp.ProcessChunk(recCtx, chunkIdx); err != nil {
			if errors.Is(err, ErrPipelineStopped) {
				record(ErrPipelineStopped)
				return
			}
			record(fmt.Errorf("(MID_26062708) %s chunk %d: %w", bp.Name(), chunkIdx, err))
		}
	}

	// Phase 1: seed chunks concurrently; do NOT wait.
	for chunkIdx := 0; chunkIdx < len(chunks); chunkIdx++ {
		wg.Add(1)
		go func(idx int) { defer wg.Done(); runOne(seed, idx) }(chunkIdx)
	}

	// Phase 2: stagger while the seed keeps running.
	if len(ordered) > 1 {
		if stagger := llmCallStagger(); stagger > 0 {
			select {
			case <-time.After(stagger):
			case <-ctx.Done():
				wg.Wait()
				return ErrPipelineStopped
			}
		}
	}

	// Phase 3: remaining (processor x chunk) concurrently, bounded.
	if len(ordered) > 1 {
		sem := make(chan struct{}, maxDocProcessorTasks(10))
		for pi := 1; pi < len(ordered); pi++ {
			for chunkIdx := 0; chunkIdx < len(chunks); chunkIdx++ {
				wg.Add(1)
				go func(bp ChunkBatchProcessor, idx int) {
					defer wg.Done()
					select {
					case sem <- struct{}{}:
					case <-ctx.Done():
						record(ErrPipelineStopped)
						return
					}
					defer func() { <-sem }()
					runOne(bp, idx)
				}(ordered[pi], chunkIdx)
			}
		}
	}

	wg.Wait()
	return firstErr
}
```

Then in `runProcessorsChunkBatched`, replace the old Phase 1/2/3 block (from the `// --- Per-chunk batching loop (two-phase) ---` comment through the closing of the Phase-3 `if len(batchProcessors) > 1 { ... wg.Wait() }`) with:

```go
	if s.Logger != nil {
		s.Logger.Info("chunk batch: scheduling three-phase cache-optimized run",
			"record_id", recordID,
			"num_processors", len(batchProcessors),
			"num_chunks", len(chunks),
		)
	}
	if schedErr := s.scheduleChunkBatch(ctx, batchProcessors, chunks, recordID); schedErr != nil {
		if errors.Is(schedErr, ErrPipelineStopped) {
			*requestStopped = true
			phaseBSpan.SetStatus(codes.Error, "stopped")
			return
		}
		if batchFirstErr == nil {
			batchFirstErr = schedErr
		}
	}
```

Remove the now-unused local `stagger := llmCallStagger()` line above (the schedule reads it internally). Ensure `sync`, `time`, `errors`, `fmt` remain imported (they already are).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run TestSeedPhaseRunsChunksConcurrently -v`
Expected: PASS.

- [ ] **Step 5: Build + vet + commit**

```bash
cd ChenWeb && go build ./server/api/doc-processing/ && go vet ./server/api/doc-processing/ && \
jj commit -m "feat(doc-proc): concurrent-seed three-phase chunk batch schedule

Replaces the sequential seed loop with a reviewer-style schedule: concurrent
seed, single LLM_CALL_STAGGER wait, bounded concurrent remainder.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Per-unit fallback — remove the all-or-nothing gate

**Files:**
- Modify: `server/api/doc-processing/chunk_batch_coordinator.go` — `runPhaseBProcessors` (currently lines ~332–371) and the batch-support check in `runProcessorsChunkBatched` (currently lines ~52–69).
- Test: `server/api/doc-processing/chunk_batch_coordinator_test.go` (extend)

**Interfaces:**
- Produces: `runPhaseBProcessors` now partitions Phase B into batch-capable vs. unsupported, runs the coordinator over batch-capable processors, and runs unsupported ones via `runProcessorsTwoPhase` **concurrently**, joining both. `runProcessorsChunkBatched` no longer falls back on the first non-batch processor; it operates only on the batch-capable subset it is given.

**Context:** Today `runProcessorsChunkBatched` loops Phase B and bails to legacy the moment one processor doesn't implement `ChunkBatchProcessor`; `runPhaseBProcessors` gates on all-or-nothing. Change both so unsupported processors don't disable batching.

- [ ] **Step 1: Write the failing test**

Add to `chunk_batch_coordinator_test.go`:

```go
// nonBatchProc implements Processor but NOT ChunkBatchProcessor.
type nonBatchProc struct{ name string; ran int32 }

func (n *nonBatchProc) Name() string { return n.name }
func (n *nonBatchProc) HandleEvent(context.Context, []byte) error {
	atomic.AddInt32(&n.ran, 1)
	return nil
}

func TestPartitionBatchProcessors(t *testing.T) {
	batch := &fakeBatchProc{name: "extract_inventory_items"}
	legacy := &nonBatchProc{name: "generate_topics"}
	got := partitionBatchProcessors([]Processor{batch, legacy})
	if len(got.batch) != 1 || got.batch[0].Name() != "extract_inventory_items" {
		t.Fatalf("batch partition wrong: %+v", got.batch)
	}
	if len(got.unsupported) != 1 || got.unsupported[0].Name() != "generate_topics" {
		t.Fatalf("unsupported partition wrong: %+v", got.unsupported)
	}
}
```

(`fakeBatchProc` is from Task 1's `chunk_batch_test.go`, same package.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run TestPartitionBatchProcessors -v`
Expected: FAIL — `partitionBatchProcessors undefined`.

- [ ] **Step 3: Implement the partition + rewire the gateway**

Add to `chunk_batch_coordinator.go`:

```go
type phaseBPartition struct {
	batch       []ChunkBatchProcessor
	unsupported []Processor
}

// partitionBatchProcessors splits Phase B processors (Phase A excluded) into
// those implementing ChunkBatchProcessor and those that don't.
func partitionBatchProcessors(processors []Processor) phaseBPartition {
	var part phaseBPartition
	for _, p := range processors {
		if p == nil || isPhaseAProcessor(p.Name()) {
			continue
		}
		if bp, ok := p.(ChunkBatchProcessor); ok {
			part.batch = append(part.batch, bp)
		} else {
			part.unsupported = append(part.unsupported, p)
		}
	}
	return part
}
```

Rewrite `runPhaseBProcessors` to run both partitions concurrently:

```go
func (s *ControlService) runPhaseBProcessors(
	ctx context.Context,
	payload []byte,
	processors []Processor,
	recordID int64,
	requestFailed, requestStopped *bool,
	firstErr *error,
	summaries *[]procResult,
) {
	part := partitionBatchProcessors(processors)

	// Only one (or zero) batch-capable processor: batching yields no
	// cross-processor cache benefit, so run everything legacy.
	if len(part.batch) <= 1 {
		s.runProcessorsTwoPhase(ctx, payload, processors, recordID,
			requestFailed, requestStopped, firstErr, summaries)
		return
	}

	var (
		wg              sync.WaitGroup
		legacyFailed    bool
		legacyStopped   bool
		legacyErr       error
		legacySummaries []procResult
	)

	// Unsupported processors run legacy-concurrent alongside the batch. Build a
	// Processor slice of just the unsupported ones plus Phase A (which
	// runProcessorsTwoPhase runs first and is idempotent/skippable when already
	// done). To avoid re-running Phase A here, pass only unsupported processors
	// and rely on Phase A having run in the batch path's chunk load.
	if len(part.unsupported) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.runProcessorsPhaseBOnly(ctx, payload, part.unsupported, recordID,
				&legacyFailed, &legacyStopped, &legacyErr, &legacySummaries)
		}()
	}

	s.runProcessorsChunkBatched(ctx, payload, processors, recordID,
		requestFailed, requestStopped, firstErr, summaries)

	wg.Wait()

	if legacyFailed {
		*requestFailed = true
		if *firstErr == nil {
			*firstErr = legacyErr
		}
	}
	if legacyStopped {
		*requestStopped = true
	}
	if summaries != nil {
		*summaries = append(*summaries, legacySummaries...)
	}
}
```

Add a Phase-B-only legacy fan-out (Phase A already ran during the batch path's chunk load), factored from `runProcessorsTwoPhase`'s Phase B block:

```go
// runProcessorsPhaseBOnly fans out the given processors concurrently (no Phase A),
// used to run batch-unsupported processors alongside the chunk-batch coordinator.
func (s *ControlService) runProcessorsPhaseBOnly(
	ctx context.Context, payload []byte, phaseB []Processor,
	recordID int64, requestFailed, requestStopped *bool, firstErr *error,
	summaries *[]procResult,
) {
	if len(phaseB) == 0 {
		return
	}
	results := make([]procResult, len(phaseB))
	var wg sync.WaitGroup
	for i, p := range phaseB {
		wg.Add(1)
		go func(i int, p Processor) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[i] = procResult{failed: true, err: fmt.Errorf("(MID_26062740) processor %q panicked: %v", p.Name(), r)}
				}
			}()
			results[i] = s.runSingleProcessorCollect(ctx, payload, p, recordID)
		}(i, p)
	}
	wg.Wait()
	if summaries != nil {
		*summaries = append(*summaries, results...)
	}
	for _, r := range results {
		if r.failed {
			*requestFailed = true
			if *firstErr == nil {
				*firstErr = r.err
			}
		}
		if r.stopped {
			*requestStopped = true
		}
	}
}
```

Finally, in `runProcessorsChunkBatched`, delete the old all-or-nothing loop (lines ~52–69 that call `runProcessorsTwoPhase` on the first non-batch processor). Instead build the batch set from the partition:

```go
	part := partitionBatchProcessors(processors)
	batchProcessors := part.batch
	if len(batchProcessors) == 0 {
		return
	}
```

(Leave the rest of `runProcessorsChunkBatched` — chunk loading, `InitChunkBatch`, `scheduleChunkBatch`, `FinalizeChunkBatch` — intact.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run 'TestPartitionBatchProcessors|TestSeedPhaseRunsChunksConcurrently' -v`
Expected: PASS.

- [ ] **Step 5: Build + vet + commit**

```bash
cd ChenWeb && go build ./server/api/doc-processing/ && go vet ./server/api/doc-processing/ && \
jj commit -m "feat(doc-proc): per-unit fallback replaces all-or-nothing batch gate

Batch-capable processors run under the coordinator; unsupported ones run
legacy-concurrent alongside instead of disabling batching for all.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Fix entity/relation batch double-processing

**Files:**
- Modify: `server/api/doc-processing/entity-relation-split.go` — add per-type batch methods on `EntityProcessor` and `RelationProcessor`.
- Modify: `server/api/doc-processing/extract-entity-relation.go` — retire/rename the combined batch methods on the embedded core so they are not inherited ambiguously.
- Test: `server/api/doc-processing/entity_relation_batch_test.go` (create)

**Interfaces:**
- Consumes: existing core helpers on `*EntityRelationProcessor`: `processChunk(ctx, recordID, chunkIdx, total, chunk, docCtx) entityRelationChunkResult` ([extract-entity-relation.go:560](../../../server/api/doc-processing/extract-entity-relation.go#L560)), `consolidateEntities`, `Store.SaveEntities`, `Store.SaveRelations`, `extractFreeformRelations(ctx, recordID, chunks, docCtx)`, `saveEntitiesToFile`, `saveRelationsToFile`, `ReindexEntitySearchForRecord`, `ReindexRelationSearchForRecord`.
- Produces: `EntityProcessor` and `RelationProcessor` each satisfy `ChunkBatchProcessor` with **non-overlapping** work (entity-only / relation-only).

**Context:** Both types embed `*EntityRelationProcessor`. Today they inherit its combined `InitChunkBatch/ProcessChunk/FinalizeChunkBatch`, so the coordinator would run entity+relation extraction twice and save twice. Rename the core's combined batch methods to unexported helpers that are no longer part of the `ChunkBatchProcessor` method set, then implement the interface explicitly on each wrapper.

- [ ] **Step 1: Write the failing test**

Create `server/api/doc-processing/entity_relation_batch_test.go`:

```go
package docprocessing

import "testing"

// The split processors must each implement ChunkBatchProcessor independently,
// and must NOT share one combined implementation via embedding.
func TestEntityAndRelationImplementBatchSeparately(t *testing.T) {
	var _ ChunkBatchProcessor = (*EntityProcessor)(nil)
	var _ ChunkBatchProcessor = (*RelationProcessor)(nil)

	// The embedded core must no longer expose ProcessChunk as part of the
	// interface (renamed away), so *EntityRelationProcessor alone is not a
	// ChunkBatchProcessor.
	if _, ok := interface{}((*EntityRelationProcessor)(nil)).(ChunkBatchProcessor); ok {
		t.Fatal("combined EntityRelationProcessor must not satisfy ChunkBatchProcessor after split")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run TestEntityAndRelationImplementBatchSeparately -v`
Expected: FAIL — currently `*EntityRelationProcessor` satisfies the interface (compile-time the last check fails, or the file won't yet have the split methods).

- [ ] **Step 3: Rename core combined batch methods; add per-type methods**

In `extract-entity-relation.go`, rename the three combined methods so they leave the interface method set:
- `InitChunkBatch` → `initEntityBatch`
- `ProcessChunk` → `processEntityChunk`
- `FinalizeChunkBatch` → `finalizeEntityBatch`

(These already do entity extraction in `ProcessChunk` and consolidate+save entities, with an optional Phase-2 window relation step in finalize. Keep the entity behavior; the window-relation step inside finalize is dropped for the split path — relations are handled by `RelationProcessor` free-form. Delete the Phase-2 window block from `finalizeEntityBatch`, keeping entity consolidation + `SaveEntities` + `saveEntitiesToFile` + `ReindexEntitySearchForRecord`.)

In `entity-relation-split.go`, add to `EntityProcessor`:

```go
func (p *EntityProcessor) InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	return p.EntityRelationProcessor.initEntityBatch(ctx, recordID, chunks, docCtx)
}
func (p *EntityProcessor) ProcessChunk(ctx context.Context, chunkIdx int) error {
	return p.EntityRelationProcessor.processEntityChunk(ctx, chunkIdx)
}
func (p *EntityProcessor) FinalizeChunkBatch(ctx context.Context) error {
	return p.EntityRelationProcessor.finalizeEntityBatch(ctx)
}
```

Add relation-only batch methods to `RelationProcessor`. Add batch state to the core struct (reuse existing `batchChunks`/`batchDocCtx`/`batchRecordID`; add `batchRelations []map[string]any` and `batchRelLang string`):

```go
func (p *RelationProcessor) InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	if strings.TrimSpace(p.RelationPromptText) == "" || p.RelationPromptErr != nil {
		p.Logger.Warn("relation prompt unavailable; relation batch skipped", "record_id", recordID)
	}
	p.batchStart = p.Now()
	p.batchRecordID = recordID
	p.batchChunks = chunks
	p.batchDocCtx = docCtx
	p.batchRelations = nil
	p.batchRelLang = "unknown"
	return nil
}

func (p *RelationProcessor) ProcessChunk(ctx context.Context, chunkIdx int) error {
	if chunkIdx < 0 || chunkIdx >= len(p.batchChunks) {
		return fmt.Errorf("(MID_26062741) %s chunk index %d out of range (len=%d)",
			p.Name(), chunkIdx, len(p.batchChunks))
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	if strings.TrimSpace(p.RelationPromptText) == "" || p.RelationPromptErr != nil {
		return nil // relation extraction disabled; nothing to do
	}
	// Free-form relations for a single chunk, sharing the canonical chunk prefix.
	rels, lang, err := p.extractFreeformRelations(ctx, p.batchRecordID,
		p.batchChunks[chunkIdx:chunkIdx+1], p.batchDocCtx)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			return ErrPipelineStopped
		}
		p.Logger.Warn("%s chunk failed", p.Name(), "record_id", p.batchRecordID, "chunk", chunkIdx, "error", err)
		return nil
	}
	if lang != "" && p.batchRelLang == "unknown" {
		p.batchRelLang = lang
	}
	p.batchRelations = append(p.batchRelations, rels...)
	return nil
}

func (p *RelationProcessor) FinalizeChunkBatch(ctx context.Context) error {
	if len(p.batchRelations) == 0 {
		return nil
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	rec, err := p.InputStore.GetInputRecord(ctx, p.batchRecordID)
	if err != nil {
		return fmt.Errorf("(MID_26062742) %s load record: %w", p.Name(), err)
	}
	createTime := p.Now().UTC().Format(time.RFC3339)
	for i := range p.batchRelations {
		p.batchRelations[i]["relation_id"] = fmt.Sprintf("%d_rel_%d", p.batchRecordID, i+1)
		p.batchRelations[i]["create_time"] = createTime
	}
	if _, err := p.Store.SaveRelations(ctx, SaveRelationsRequest{
		InputRecordID: p.batchRecordID,
		EventID:       eventIDFromContext(ctx),
		Language:      firstNonEmptyTrimmed(p.batchRelLang, "unknown"),
		ModelName:     p.ModelName,
		PromptName:    p.RelationPromptRef,
		Relations:     p.batchRelations,
	}); err != nil {
		return fmt.Errorf("(MID_26062743) %s save relations: %w", p.Name(), err)
	}
	if fileErr := p.saveRelationsToFile(p.batchRecordID, rec, p.batchRelations); fileErr != nil {
		p.Logger.Warn("save relations to file failed", "record_id", p.batchRecordID, "error", fileErr)
	}
	if reErr := ReindexRelationSearchForRecord(ctx, p.batchRecordID, p.Logger); reErr != nil {
		p.Logger.Warn("reindex relation search failed", "record_id", p.batchRecordID, "error", reErr)
	}
	return nil
}
```

Add the two new fields to the `EntityRelationProcessor` struct (near the existing batch-state fields at line ~62):

```go
	batchRelations []map[string]any
	batchRelLang   string
```

Verify `extractFreeformRelations` accepts a `[]Chunk` slice (it does — see [extract-entity-relation.go:650](../../../server/api/doc-processing/extract-entity-relation.go#L650) region; if its signature differs, adapt the single-chunk call accordingly, passing the one chunk).

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run TestEntityAndRelationImplementBatchSeparately -v`
Expected: PASS.

- [ ] **Step 5: Build + vet + commit**

```bash
cd ChenWeb && go build ./server/api/doc-processing/ && go vet ./server/api/doc-processing/ && \
jj commit -m "fix(doc-proc): split entity/relation batch methods to stop double-processing

EntityProcessor does entity-only; RelationProcessor does free-form relations
only. The combined core batch methods are renamed out of the interface set.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 5: `extract_metrics` — ChunkBatchProcessor (2-pass)

**Files:**
- Modify: `server/api/doc-processing/extract-metrics.go`
- Test: `server/api/doc-processing/extract_metrics_batch_test.go` (create)

**Interfaces:**
- Consumes: `canonicalChunkInputText`, `metricCandidateTask(p.MentionPromptText)`, `p.extractMetricCandidatePayloadWithFallback(ctx, inputText, taskPrompt)`, `normalizeMetricCandidateMentions`, `groupCandidatesByChunk(candidates, p.MetricEnrichGroupSize)`, the Pass-2 enrichment loop body (extract-metrics.go ~805–860), `p.Store.SaveMetrics`, `p.saveMetricsToFile`. Metrics indexing stays in Phase C `PostProcessIndex` (unchanged).
- Produces: `MetricsProcessor` satisfies `ChunkBatchProcessor`.

**Context:** `pass1Result` is a local type inside `extractMetricsFromChunksWithLLM` (line 690). Promote it to package level so batch methods can accumulate candidates across `ProcessChunk` calls, then run Pass 2 + save in `FinalizeChunkBatch`.

- [ ] **Step 1: Write the failing test**

Create `server/api/doc-processing/extract_metrics_batch_test.go`:

```go
package docprocessing

import "testing"

func TestMetricsProcessorImplementsChunkBatch(t *testing.T) {
	var _ ChunkBatchProcessor = (*MetricsProcessor)(nil)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run TestMetricsProcessorImplementsChunkBatch -v`
Expected: FAIL — compile error: `*MetricsProcessor` does not implement `ChunkBatchProcessor`.

- [ ] **Step 3: Promote pass1Result; add batch state + methods**

At package scope in `extract-metrics.go`, add:

```go
// metricsPass1Result is the per-chunk candidate-extraction output, promoted from
// the local type in extractMetricsFromChunksWithLLM so batch methods can share it.
type metricsPass1Result struct {
	mentions    []metricCandidateMention
	language    string
	modelName   string
	didFallback bool
}
```

Replace the local `type pass1Result struct {...}` in `extractMetricsFromChunksWithLLM` with uses of `metricsPass1Result` (rename references in that function accordingly; behavior identical).

Add batch-state fields to `MetricsProcessor` (near its struct definition):

```go
	batchRecordID   int64
	batchChunks     []Chunk
	batchDocCtx     string
	batchCandidates []metricCandidate
	batchLang       string
	batchModelName  string
	batchStart      time.Time
```

Add the interface methods:

```go
func (p *MetricsProcessor) InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	if p.MentionPromptErr != nil {
		return fmt.Errorf("(MID_26062750) %s candidate prompt error: %w", p.Name(), p.MentionPromptErr)
	}
	if p.ModelErr != nil {
		p.Logger.Warn("%s skipped: model config error", p.Name(), "record_id", recordID, "error", p.ModelErr)
		return nil
	}
	p.batchStart = p.Now()
	p.batchRecordID = recordID
	p.batchChunks = chunks
	p.batchDocCtx = docCtx
	p.batchCandidates = nil
	p.batchLang = "unknown"
	p.batchModelName = strings.TrimSpace(p.MentionModelName)
	return nil
}

func (p *MetricsProcessor) ProcessChunk(ctx context.Context, chunkIdx int) error {
	if chunkIdx < 0 || chunkIdx >= len(p.batchChunks) {
		return fmt.Errorf("(MID_26062751) %s chunk index %d out of range (len=%d)", p.Name(), chunkIdx, len(p.batchChunks))
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	chunk := p.batchChunks[chunkIdx]
	block := chunksToBlocks([]Chunk{chunk})[0]
	inputText := canonicalChunkInputText(chunk.Lines, p.batchDocCtx)
	taskPrompt := metricCandidateTask(p.MentionPromptText)
	callStart := p.Now()
	callID := fmt.Sprintf("%d_p1_c%d", p.batchRecordID, chunkIdx)
	payload, modelName, err := p.extractMetricCandidatePayloadWithFallback(ctx, inputText, taskPrompt)
	p.logExtractMetricsChunk(ctx, p.batchRecordID, callID, block.Index, len(p.batchChunks),
		[]string{strings.TrimSpace(modelName)}, p.MentionPromptRef, payload, err, callStart, p.Now(), "")
	if err != nil {
		if isCtxStopped(ctx) {
			return ErrPipelineStopped
		}
		p.Logger.Warn("%s chunk failed", p.Name(), "record_id", p.batchRecordID, "chunk", chunkIdx, "error", err)
		return nil
	}
	lang := ApiUtils.NormalizeLang(asString(payload["language"]))
	if lang == "chinese" || lang == "中文" || lang == "zh-cn" {
		lang = "zh"
	}
	if lang != "" && p.batchLang == "unknown" {
		p.batchLang = lang
	}
	if m := strings.TrimSpace(modelName); m != "" {
		p.batchModelName = m
	}
	raw, _ := payload["candidates"].([]any)
	mentions := normalizeMetricCandidateMentions(raw, block)
	for _, mention := range mentions {
		p.batchCandidates = append(p.batchCandidates, metricCandidate{ChunkIndex: chunkIdx, Mention: mention})
	}
	return nil
}

func (p *MetricsProcessor) FinalizeChunkBatch(ctx context.Context) error {
	if len(p.batchCandidates) == 0 {
		p.Logger.Info("%s batch: no candidates", p.Name(), "record_id", p.batchRecordID)
		return nil
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	metrics, err := p.enrichMetricCandidates(ctx, p.batchRecordID, p.batchCandidates, p.batchDocCtx)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			return ErrPipelineStopped
		}
		return fmt.Errorf("(MID_26062752) %s enrich metrics: %w", p.Name(), err)
	}
	rec, err := p.InputStore.GetInputRecord(ctx, p.batchRecordID)
	if err != nil {
		return fmt.Errorf("(MID_26062753) %s load record: %w", p.Name(), err)
	}
	if _, err := p.Store.SaveMetrics(ctx, SaveMetricsRequest{
		InputRecordID: p.batchRecordID,
		EventID:       eventIDFromContext(ctx),
		Language:      firstNonEmptyTrimmed(p.batchLang, "unknown"),
		ModelName:     firstNonEmptyTrimmed(p.batchModelName, p.MentionModelName),
		PromptName:    p.MentionPromptRef,
		Metrics:       metrics,
	}); err != nil {
		return fmt.Errorf("(MID_26062754) %s save metrics: %w", p.Name(), err)
	}
	if fileErr := p.saveMetricsToFile(p.batchRecordID, rec, metrics); fileErr != nil {
		p.Logger.Warn("save metrics to file failed", "record_id", p.batchRecordID, "error", fileErr)
	}
	// Indexing runs in Phase C via PostProcessIndex (unchanged).
	return nil
}
```

Extract the existing Pass-2 enrichment body (extract-metrics.go ~805–870) into a reusable method `enrichMetricCandidates(ctx, recordID int64, candidates []metricCandidate, docCtx string) ([]map[string]any, error)` and call it from both `extractMetricsFromChunksWithLLM` (to avoid duplicating logic — DRY) and `FinalizeChunkBatch`. Match `SaveMetricsRequest` fields to its actual definition ([extract-metrics.go:106](../../../server/api/doc-processing/extract-metrics.go#L106)); adjust field names if they differ from the sketch above.

Confirm `extract-metrics.go` imports include `ApiUtils` and `time` (they do — used elsewhere in the file).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run TestMetricsProcessorImplementsChunkBatch -v`
Expected: PASS.

- [ ] **Step 5: Build + vet + commit**

```bash
cd ChenWeb && go build ./server/api/doc-processing/ && go vet ./server/api/doc-processing/ && \
jj commit -m "feat(doc-proc): extract_metrics implements ChunkBatchProcessor (2-pass)

Pass 1 candidate extraction per chunk accumulates; Pass 2 enrichment + save in
FinalizeChunkBatch. Indexing stays in Phase C.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 6: `extract_semantic_projections` — ChunkBatchProcessor (2-pass)

**Files:**
- Modify: `server/api/doc-processing/extract-semantic-projections.go`
- Test: `server/api/doc-processing/extract_semantic_projections_batch_test.go` (create)

**Interfaces:**
- Consumes: the processor's existing candidate + enrich per-chunk functions (the two-pass body inside its `HandleEvent`/extract path), `canonicalChunkInputText`, `p.Store.SaveSemanticProjections` ([extract-semantic-projections.go:61](../../../server/api/doc-processing/extract-semantic-projections.go#L61)).
- Produces: `SemanticProjectionsProcessor` satisfies `ChunkBatchProcessor`; `ProcessChunk` runs Pass 1 then Pass 2 for one chunk within a single call.

**Context:** Mirror Task 5, but both passes happen inside `ProcessChunk` (Pass 2 enrich immediately follows Pass 1 for the same chunk so the cached prefix is reused while warm). Accumulate enriched projections; save all in `FinalizeChunkBatch`.

- [ ] **Step 1: Write the failing test**

Create `server/api/doc-processing/extract_semantic_projections_batch_test.go`:

```go
package docprocessing

import "testing"

func TestSemanticProjectionsImplementsChunkBatch(t *testing.T) {
	var _ ChunkBatchProcessor = (*SemanticProjectionsProcessor)(nil)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run TestSemanticProjectionsImplementsChunkBatch -v`
Expected: FAIL — compile error: does not implement `ChunkBatchProcessor`.

- [ ] **Step 3: Add batch state + methods**

First, factor the existing per-chunk two-pass work into a method the batch path can call:

```go
// projectChunk runs candidate extraction (Pass 1) then enrichment (Pass 2) for a
// single chunk, sharing the canonical cached prefix, and returns enriched rows.
func (p *SemanticProjectionsProcessor) projectChunk(ctx context.Context, recordID int64, chunkIdx int, chunk Chunk, docCtx string) ([]map[string]any, string, error) {
	// Move the body of the existing per-chunk closure (candidate call + enrich
	// call) here; return enriched projection rows + detected language.
}
```

Refactor the existing extract path (in `HandleEvent`/its `runConcurrent` closure) to call `projectChunk` so logic is not duplicated (DRY).

Add batch-state fields to `SemanticProjectionsProcessor`:

```go
	batchRecordID    int64
	batchChunks      []Chunk
	batchDocCtx      string
	batchProjections []map[string]any
	batchLang        string
	batchStart       time.Time
```

Add the interface methods:

```go
func (p *SemanticProjectionsProcessor) InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error {
	if p.CandidatePromptErr != nil {
		return fmt.Errorf("(MID_26062760) %s candidate prompt error: %w", p.Name(), p.CandidatePromptErr)
	}
	if p.EnrichPromptErr != nil {
		return fmt.Errorf("(MID_26062761) %s enrich prompt error: %w", p.Name(), p.EnrichPromptErr)
	}
	p.batchStart = p.Now()
	p.batchRecordID = recordID
	p.batchChunks = chunks
	p.batchDocCtx = docCtx
	p.batchProjections = nil
	p.batchLang = "unknown"
	return nil
}

func (p *SemanticProjectionsProcessor) ProcessChunk(ctx context.Context, chunkIdx int) error {
	if chunkIdx < 0 || chunkIdx >= len(p.batchChunks) {
		return fmt.Errorf("(MID_26062762) %s chunk index %d out of range (len=%d)", p.Name(), chunkIdx, len(p.batchChunks))
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	rows, lang, err := p.projectChunk(ctx, p.batchRecordID, chunkIdx, p.batchChunks[chunkIdx], p.batchDocCtx)
	if err != nil {
		if errors.Is(err, ErrPipelineStopped) {
			return ErrPipelineStopped
		}
		p.Logger.Warn("%s chunk failed", p.Name(), "record_id", p.batchRecordID, "chunk", chunkIdx, "error", err)
		return nil
	}
	if lang != "" && p.batchLang == "unknown" {
		p.batchLang = lang
	}
	p.batchProjections = append(p.batchProjections, rows...)
	return nil
}

func (p *SemanticProjectionsProcessor) FinalizeChunkBatch(ctx context.Context) error {
	if len(p.batchProjections) == 0 {
		return nil
	}
	if isCtxStopped(ctx) {
		return ErrPipelineStopped
	}
	if _, err := p.Store.SaveSemanticProjections(ctx, SaveSemanticProjectionsRequest{
		InputRecordID: p.batchRecordID,
		EventID:       eventIDFromContext(ctx),
		Language:      firstNonEmptyTrimmed(p.batchLang, "unknown"),
		ModelName:     p.EnrichModelName,
		PromptName:    p.EnrichPromptRef,
		Projections:   p.batchProjections,
	}); err != nil {
		return fmt.Errorf("(MID_26062763) %s save projections: %w", p.Name(), err)
	}
	return nil
}
```

Match `SaveSemanticProjectionsRequest` field names to its actual definition ([extract-semantic-projections.go:68](../../../server/api/doc-processing/extract-semantic-projections.go#L68)); adjust if they differ. Preserve any existing artifact-write/indexing the legacy save path performed by calling the same helper here.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ -run TestSemanticProjectionsImplementsChunkBatch -v`
Expected: PASS.

- [ ] **Step 5: Build + vet + commit**

```bash
cd ChenWeb && go build ./server/api/doc-processing/ && go vet ./server/api/doc-processing/ && \
jj commit -m "feat(doc-proc): extract_semantic_projections implements ChunkBatchProcessor

Both passes run per chunk within one ProcessChunk call to reuse the warm cached
prefix; FinalizeChunkBatch saves all accumulated projections.

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 7: Full-package verification + ADR/docs update

**Files:**
- Modify: `KnowledgeStore/doc-repo/adrs/202606/2026062701-adr-deepseek-cache-doc-processors.md`
- Verify only: whole package.

**Interfaces:** none (docs + verification).

- [ ] **Step 1: Run the full package test suite**

Run: `cd ChenWeb && go test ./server/api/doc-processing/ 2>&1 | tail -30`
Expected: no net-new failures vs. the pre-change baseline (the package has known pre-existing expectation-drift failures noted in ADR 2026062501 "Verification"). Record which failures are pre-existing.

- [ ] **Step 2: Build + vet the workspace-affected packages**

Run: `cd ChenWeb && go build ./server/... && go vet ./server/api/doc-processing/`
Expected: clean.

- [ ] **Step 3: Update the ADR**

In `2026062701-adr-deepseek-cache-doc-processors.md`:
- Rewrite the Phase 4/5 execution description to the **actual** algorithm: task-based three-phase schedule (concurrent seed → single `LLM_CALL_STAGGER` → bounded concurrent remainder), seed forced to a 1-pass processor, per-unit fallback (unsupported processors run legacy-concurrent alongside — no all-or-nothing gate).
- Update the "ChunkBatchProcessor implementation status" table: `extract_metrics` ✅, `extract_semantic_projections` ✅, `extract_entity` ✅ (entity-only), `extract_relation` ✅ (free-form, relation-only), `extract_provisions` ✅, `extract_inventory_items` ✅; `generate_topics` / `generate_scene_blocks` deferred (follow-up spec); `extract_products` out of scope.
- Add the "Known issues" correction: the registered processors are the split `EntityProcessor`/`RelationProcessor` (not the combined `extract_entity_relation`); the combined batch methods were renamed out of the interface set to stop double-processing.
- Add `MAX_DOC_PROCESSOR_TASKS` (default 10) to the env-var table.
- Add a line noting the algorithm now matches the doc-reviewers scheduler (`review_cache_scheduler.go`), and link this plan + the design spec.

- [ ] **Step 4: Commit**

```bash
cd ChenWeb && jj commit -m "docs(adr): update 2026062701 for unified processor cache algorithm

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

(The ADR lives in the `KnowledgeStore` repo, which is separate from `ChenWeb`. Commit it in that repo: `cd KnowledgeStore && jj commit -m "..."` — keep application and KnowledgeStore commits separate.)

---

## Self-Review Notes

- **Spec coverage:** §3.1 → Tasks 1–2; §3.2 → Task 1 + Task 2 (semaphore); §3.3 → Task 3; §3.4 → Task 1 (`orderBatchProcessorsSeedFirst`); §3.5 → Task 4; §3.6 → Tasks 5–6; §6 docs → Task 7. Testing (§5) is embedded per task + Task 7 full run.
- **Deferred:** `generate_topics`, `generate_scene_blocks`, shared-scheduler extraction, `extract_products` — explicitly out of scope; handled by per-unit fallback.
- **Adaptation note:** save-request struct field names (`SaveMetricsRequest`, `SaveSemanticProjectionsRequest`, `SaveRelationsRequest`) and `extractFreeformRelations`'s exact signature must be matched to the real definitions during implementation; the plan references their file locations. This is deliberate — verify at the call site, don't invent fields.
```
