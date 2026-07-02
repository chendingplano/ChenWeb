# Design: Unify DeepSeek Cache Optimization Across Doc Processors

Date: 2026-07-01

Extends: [ADR 2026062701 — DeepSeek Prompt Cache for Doc Processors](../../../../KnowledgeStore/doc-repo/adrs/202606/2026062701-adr-deepseek-cache-doc-processors.md)

Related: [ADR 2026062501 — DeepSeek Prompt Cache for Document Reviewers](../../../../KnowledgeStore/doc-repo/adrs/202606/2026062501-adr-deepseek-cache.md)

## 1. Problem

The doc-processing pipeline was supposed to run all chunk-consuming processors under
a per-chunk batching coordinator so that LLM calls sharing an identical chunk prefix
arrive at DeepSeek back-to-back and reuse the implicit prompt cache. In practice the
optimization never activates and, if it did, it would be wrong:

1. **All-or-nothing gate never passes.** `runPhaseBProcessors` only uses the
   coordinator when *every* Phase B processor implements `ChunkBatchProcessor`. Only
   3 of the configured processors do (`extract_provisions`, `extract_inventory_items`,
   and the unused combined `extract_entity_relation`), so the full pipeline always
   falls back to legacy concurrent mode — the exact scatter-in-time anti-pattern the
   optimization was meant to remove.

2. **Sequential seed phase.** Even when the coordinator runs, Phase 1 (seed) processes
   the seed processor's chunks in a serial `for` loop, an unnecessary ~N× bottleneck
   (chunks have distinct prefixes, so there is no cross-chunk cache benefit to
   serializing them).

3. **Entity/Relation double-processing bug.** The pipeline registers the *split*
   `EntityProcessor` ("extract_entity") and `RelationProcessor` ("extract_relation"),
   both embedding a shared `*EntityRelationProcessor`. The batch methods
   (`InitChunkBatch`/`ProcessChunk`/`FinalizeChunkBatch`) exist only on the embedded
   core and run the *old combined* entity+relation extraction. Activating the
   coordinator would extract and save entities and relations **twice** (and via a
   different relation method than the split path actually uses).

4. **Divergent from the reviewers.** The document **reviewers** already implement the
   intended algorithm well — a task-based, three-phase, concurrent-seed scheduler with
   graceful per-reviewer fallback ([review_cache_scheduler.go](../../../server/api/doc-reviews/review_cache_scheduler.go)).
   The processors should use the same algorithm.

## 2. Goal & Scope

**Goal:** Make the doc processors use the same cache-optimization scheduling algorithm
as the document reviewers, correctly and by default, for all six currently-configured
chunk-based processors.

**In scope (this spec):**

- Rewrite the doc-processing coordinator scheduling to match the reviewers' three-phase
  algorithm (concurrent seed → stagger → concurrent remainder), with per-unit fallback.
- Convert / correct these six processors to the `ChunkBatchProcessor` lifecycle:
  `extract_metrics`, `extract_semantic_projections`, `extract_provisions` (done),
  `extract_inventory_items` (done), `extract_entity`, `extract_relation`.
- Update ADR 2026062701.

**Out of scope (follow-up spec):**

- `generate_topics` rework (decouple from the semantic-chunking/segmentation step so it
  consumes the shared `.chunks` like the other processors).
- `generate_scene_blocks` batch conversion.
- Extracting a single shared scheduler used by *both* processors and reviewers (the
  reviewers keep their existing implementation; the processor side mirrors the pattern).
- `extract_products` (intentionally unused).

Both deferred processors are handled gracefully by the per-unit fallback (Section 3.3)
until their follow-up: they run legacy-concurrent alongside the batch without disabling it.

## 3. Design

### 3.1 Unified scheduling algorithm (coordinator rewrite)

Rewrite `runProcessorsChunkBatched` in
[chunk_batch_coordinator.go](../../../server/api/doc-processing/chunk_batch_coordinator.go)
so its scheduling mirrors [runReviewTasksForPromptCache](../../../server/api/doc-reviews/review_cache_scheduler.go).
The `ChunkBatchProcessor` interface is unchanged:

```go
type ChunkBatchProcessor interface {
    Name() string
    InitChunkBatch(ctx context.Context, recordID int64, chunks []Chunk, docCtx string) error
    ProcessChunk(ctx context.Context, chunkIdx int) error
    FinalizeChunkBatch(ctx context.Context) error
}
```

Execution per document:

1. Load shared chunks + doc context once (unchanged).
2. `InitChunkBatch` on each batch processor (unchanged).
3. **Order processors so the seed (`batchProcessors[0]`) is a 1-pass processor.** The
   seed plants each chunk's prefix with a single clean LLM call. 2-pass processors
   (`metrics`, `semantic_projections`) must not be the seed. The coordinator sorts
   batch processors so a 1-pass processor is first (see 3.4), rather than relying on
   registration order.
4. **Phase 1 — seed, concurrent.** Fire the seed processor's chunk tasks
   (`ProcessChunk(0..N-1)`) as goroutines under a shared `WaitGroup` (`allWg`); **do
   not wait**. This replaces the current serial `for` loop.
5. **Phase 2 — stagger.** `select { case <-time.After(llmCallStagger()): case
   <-ctx.Done(): ... }`. Seeds continue running during the wait (faithful to the
   reviewers). Skipped when there is only one batch processor.
6. **Phase 3 — remainder, concurrent.** Fire all `(processor, chunkIdx)` tasks for
   `batchProcessors[1..M]` as goroutines under `allWg`, **bounded by a `maxTasks`
   semaphore** (new; caps `(M-1)×N` concurrent calls — addresses ADR "remaining work"
   item #6).
7. `allWg.Wait()`.
8. `FinalizeChunkBatch` on every batch processor (save/index — only after all
   `ProcessChunk` complete). Unchanged.

Stagger semantics and `llmCallStagger()` (env `LLM_CALL_STAGGER`, default 1s) are
unchanged.

### 3.2 Concurrency cap

Phase 3 concurrency is bounded by a `maxTasks` semaphore, read from a new env var
`MAX_DOC_PROCESSOR_TASKS` (mirrors the reviewers' `MAX_DOC_REVIEWER_TASKS`), defaulting
to a sensible value (e.g. the existing `MaxDocProcessPipelines` default of 10). Phase 1
seeds are unbounded (one processor × N chunks), matching the reviewers.

### 3.3 Per-unit fallback (drops the all-or-nothing gate)

`runPhaseBProcessors` partitions Phase B processors into:

- `batchProcessors` — those implementing `ChunkBatchProcessor`.
- `unsupported` — those that don't.

It runs the batch coordinator over `batchProcessors` **and** runs `unsupported`
concurrently via the existing legacy fan-out, in parallel, joining both before Phase C.
This mirrors the reviewers' `unsupported` + `runReviewersLegacy` split. Effects:

- The all-or-nothing gate is removed; the optimization activates for the processors that
  support it regardless of the others.
- Deferred `generate_topics` / `generate_scene_blocks` (and any future non-batch
  processor) no longer disable the optimization.

### 3.4 Seed selection

Add a helper that, given the batch processors, returns them ordered with a 1-pass
processor first. A processor advertises whether it is single-pass via a small predicate
(either a new optional interface method, e.g. `IsMultiPass() bool`, or a name-based set
maintained next to the coordinator). Recommendation: a name-based set in the coordinator
(`multiPassProcessors = {"extract_metrics","extract_semantic_projections"}`) to avoid
widening the interface; revisit if the set grows. If no 1-pass processor is present, fall
back to the first processor (the seed is still valid, just 2-pass).

### 3.5 Entity/Relation split fix

Give `EntityProcessor` and `RelationProcessor` their own batch entrypoints so each does
only its half; the shared `*EntityRelationProcessor` keeps the extraction/save helpers.

- `EntityProcessor.ProcessChunk` — Phase 1 entity extraction for the chunk; accumulate.
  `EntityProcessor.FinalizeChunkBatch` — consolidate + save entities. Phase C indexing
  unchanged (`PostProcessIndex`).
- `RelationProcessor.ProcessChunk` — free-form relation extraction for the chunk
  (the `extractFreeformRelations` path the split `RelationProcessor` actually uses
  today — **not** the window-based relations the old combined batch used); accumulate.
  `RelationProcessor.FinalizeChunkBatch` — save relations. Endpoint linking stays in
  Phase C (`PostProcessIndex`).
- Both share the same canonical chunk prefix, so entity and relation calls on the same
  chunk still cache-hit each other.
- Remove/retire the combined batch methods on the embedded core so they cannot be
  inherited ambiguously (the combined `extract_entity_relation` processor is not
  registered in the pipeline).

### 3.6 2-pass processor conversion

Both processors already build `canonicalChunkInputText` per chunk; the work is moving
from `runConcurrent`-over-all-chunks into the lifecycle and promoting local types to
package level.

**`extract_metrics`** (ADR hand-off note #1):

- Promote the local `pass1Result` (extract-metrics.go:690) to a package-level
  accumulator type; add batch-state fields (`batchRecordID`, `batchChunks`,
  `batchDocCtx`, `batchCandidates []metricCandidate`, `batchStart`).
- `InitChunkBatch` — validate prompts/models; reset accumulators.
- `ProcessChunk(i)` — run Pass 1 (candidate extraction) for chunk `i` using the existing
  per-chunk closure body; append candidates. Keep per-chunk `logExtractMetricsChunk`
  logging + cache-token stamping.
- `FinalizeChunkBatch` — run Pass 2 (enrichment) on accumulated candidates
  (`groupCandidatesByChunk` is already per-chunk), then save to `kb.metrics` + write the
  `.metrics` artifact. Metrics indexing remains in Phase C `PostProcessIndex`.

**`extract_semantic_projections`** (ADR hand-off note #2):

- Add batch-state fields; accumulator holds per-chunk enriched projections.
- `ProcessChunk(i)` — run Pass 1 (candidate) **then** Pass 2 (enrich) for chunk `i`
  **within the one call**, keeping the two calls adjacent so the chunk's cached prefix
  is reused while warm.
- `FinalizeChunkBatch` — save all accumulated projections (+ existing indexing path).

### 3.7 Components & boundaries

| Unit | Responsibility | Interface |
|---|---|---|
| `runProcessorsChunkBatched` | Three-phase schedule over batch processors | inputs: batch processors, chunks; calls `ChunkBatchProcessor` |
| `runPhaseBProcessors` | Partition batch vs. unsupported; run both, join | gateway from `control.go` |
| seed-ordering helper | Put a 1-pass processor first | pure function over `[]ChunkBatchProcessor` |
| `maxTasks` semaphore | Bound Phase 3 concurrency | env `MAX_DOC_PROCESSOR_TASKS` |
| Each processor's batch methods | Per-chunk LLM call + accumulate; finalize saves | `ChunkBatchProcessor` |

## 4. Error handling

- First error wins (`batchFirstErr`); other processors continue so their results still
  finalize.
- `ErrPipelineStopped` / `ctx.Done()` honored in every phase (seed, stagger, remainder,
  finalize); `requestStopped` set on cancel; in-flight goroutines exit via `ctx` checks
  and are joined by `allWg` before return.
- Per-chunk `ProcessChunk` failure is logged and recorded but does not abort the batch
  (accumulate what succeeds), matching current behavior.
- `unsupported` processors' errors are merged into the same first-error/summary path.

## 5. Testing

- **Scheduler unit tests** (fake processors): seed tasks start concurrently (not
  serialized); stagger is honored / skipped for single processor; Phase 3 respects the
  `maxTasks` cap; per-unit partition routes non-implementers to legacy; stop mid-phase
  (seed / stagger / remainder) exits cleanly and joins goroutines.
- **Per-processor equivalence tests**: `ProcessChunk` + `FinalizeChunkBatch` produce the
  same DB rows / artifacts as the legacy `HandleEvent` path for `metrics`,
  `semantic_projections`, `extract_entity`, `extract_relation` (no double-save for
  entity/relation).
- **Build/vet**: `go build ./server/api/doc-processing/`,
  `go test ./server/api/doc-processing/` (no net-new failures vs. baseline),
  `go vet ./...`.
- **Manual/A-B**: run a real record with `LLM_CALL_STAGGER=0` vs. `5`; inspect
  `kb.doc_proc_logs` cache counters — first pass on a chunk shows misses, subsequent
  processors on the same chunk show climbing hits.

## 6. Documentation updates

- Update ADR 2026062701: correct the coordinator description (task-based, concurrent
  seed, per-unit fallback, `maxTasks` cap), fix the status table (metrics /
  semantic_projections / entity / relation done; topics / scene_blocks deferred;
  products out), record the entity/relation split fix, and note the algorithm now
  matches the doc-reviewers scheduler.
- Note the new `MAX_DOC_PROCESSOR_TASKS` env var in the ADR env table.

## 7. Risks & mitigations

- **Seed still running when remainder fires** — accepted (same as reviewers); the stagger
  targets prefix persistence, not seed completion. If cache hit rates disappoint, the
  fallback is to wait for the seed processor's chunks before the stagger (a one-line
  change), but default to the reviewer-faithful behavior.
- **2-pass restructure regressions** — covered by equivalence tests before switching the
  registered processors' active path.
- **Concurrency spikes** — bounded by `MAX_DOC_PROCESSOR_TASKS`.
