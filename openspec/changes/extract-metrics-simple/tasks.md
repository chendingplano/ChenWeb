## 1. Shared tail (no behaviour change)

- [x] 1.1 Extract the post-LLM tail of `enrichMetricCandidates` into `finishMetricRows`; `enrichMetricCandidates` calls it → verify: existing `doc-processing` tests pass unchanged
- [x] 1.2 Add the `name` and `simple` fields to `MetricsProcessor`; `Name()` returns `name` when set

## 2. Prompt

- [x] 2.1 Write `prompts/prompt-extract-metric-simple-v1.md` from candidates v30 and enrich v29, with the compact output schema → verify: at most about 30 KB; every drop reason and enum of v29 present

## 3. Simple extraction

- [x] 3.1 `NewMetricsSimpleProcessor`: prompt env `EXTRACT_METRICS_SIMPLE_PROMPT`, model env `EXTRACT_METRICS_SIMPLE_MODEL_NAME` → `ENRICH_METRICS_MODEL_NAME` → `EXTRACT_METRICS_MODEL_NAME`, thinking disabled
- [x] 3.2 Compact-row expansion and code-built fields (`expandSimpleMetricRow`, unit table, section/context from chunk lines, `candidate_id`) → verify: unit tests for the spec scenarios
- [x] 3.3 Set-aside items and `invalid_row` → `droppedMetricRow` (stage `enrich`); overlap-only rows discarded → verify: unit tests
- [x] 3.4 Per-chunk call: system-message layout, structured-output contract requiring `m`, halves retry → verify: test with a fake extractor checks the messages are task-first and identical across chunks, and the retry marks the other half `o`
- [x] 3.5 Warm-up gate → verify: tests for the cold (waits until 60 s after start, with an injectable clock/sleep) and warm (no wait) cases
- [x] 3.6 Wire the simple branch into the sequential path (`extractMetricsFromChunksWithLLM`) and the chunk-batch path (`InitChunkBatch`, `ProcessChunk`, `FinalizeChunkBatch`), then `finishMetricRows` → verify: end-to-end test with a fake extractor saves rows and drops through the fake store
- [x] 3.7 Per-chunk `kb.doc_proc_logs` entry (activity `extract_metrics_simple`) with the raw payload

## 4. Registration

- [x] 4.1 `productionProcessorSpecs`, runtime processor list, runtime env-key map, services map keyed by `Name()`, runtime overrides for the two new env keys
- [x] 4.2 A run that selects `extract_metrics` + `extract_metrics_simple` is refused (`metricProcessorConflict` in `ControlService`) → verify: unit test; both configured still builds the runtime
- [x] 4.3 Name lists: `requiresChunkingDependency`, `processorRequiresLLM`, non-seed processors, `normalize_assertions` Phase C dependency
- [x] 4.4 `config.local.toml`: add to `required_processors` (local only, not committed)

## 5. Verification and docs

- [x] 5.1 `go build ./...`, `go vet` and `go test ./server/api/doc-processing/...` pass, except `TestKBMetricsReadersExcludeResolvedRows`, which already failed before this change (it flags `kbhandler/question_generator_worker.go`)
- [x] 5.2 Live in-process run on record 416 with the real model (no database writes): calls, cache hits, output tokens and rows compared with extract_metrics run 237
- [x] 5.3 `score_io.py`: `fetch_extraction_logs` accepts both processor names (`.agents` repo)
- [x] 5.4 Capsule `KnowledgeStore/Capsules/coding-capsules/doc-processor/extract-metrics-simple-spec.md`, and a pointer to it from `extract-metrics-spec.md`
- [ ] 5.5 Benchmark: run each processor through doc-processor with `force_clear` on the gold records (416, 753, 371, 364, 376), score both, compare costs from `llm_usage_event` (first and second run)
