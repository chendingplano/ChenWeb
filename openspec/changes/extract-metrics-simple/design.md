## Context

`extract_metrics` (capsule `KnowledgeStore/Capsules/coding-capsules/doc-processor/extract-metrics-spec.md`)
runs two LLM passes. Pass 1 lists candidates per chunk with `prompt-extract-metric-candidates-v30.md`
(40 KB); Pass 2 enriches them in batches of five with `prompt-enrich-metrics-v29.md` (70 KB). Both
use the document-first layout (`DocumentFirst = true`): a constant one-line system message, then
one user message with the chunk followed by the instructions. Everything after the enrich calls is
deterministic code, plus merge-resolve calls in merge mode and an open-value decision model that
receives no rows today.

Measured on runs 240 (record 800) and 237 (record 416), 2026-10-19, DeepSeek flash off-peak
prices (cache hit ¥0.02, miss ¥1, output ¥4 per million tokens):

| | Record 800 | Record 416 |
|---|---|---|
| Calls | 17 candidates + 32 enrich | 10 + 9 |
| Input cost | ¥0.80 | ¥0.27 |
| Output cost | ¥0.60 | ¥0.07 |

Output per stored row is about 630 tokens (126 candidate + 504 enrich). Field shares of the enrich
output: key names 31%, English text 32%, `objects` 20%, almost-always-empty fields 11%.

Facts about the code that shape the design:

- `MetricsProcessor` serves two entry points: `HandleEvent` (sequential) and the chunk-batch trio
  `InitChunkBatch` / `ProcessChunk` / `FinalizeChunkBatch`, which production uses
  (`RUN_DOC_PROCESSOR_CONCURRENT` defaults to true).
- Logs, statuses and set-aside rows name the processor through `p.Name()`.
- With `DocumentFirst = false` the shared client sends the prompt as the system message and the
  document as the user message (`shared/go/api/llm/openai_client.go`, `buildMessages`). No shared
  code changes are needed for the cacheable layout.
- The coordinator sends chunk 0 of its seed processor alone, then waits `LLM_CALL_STAGGER`
  (default 1 s) before the rest. DeepSeek needs about 60 s before a new prefix is reusable
  (measured 2026-08-16), so this wait warms nothing.
- `normalizeArtifactObjectsForArtifact` already builds a metric's object from `subject` when a row
  has no `objects`.
- `applyTableMetricContextsForRecord` already replaces the context of table rows in code.
- `score_io.py` and the in-app Benchmark page score the record's current `kb.metrics` and
  `kb.metrics_dropped` rows, whatever wrote them.

## Goals / Non-Goals

**Goals:**

- A processor that produces `kb.metrics` rows the benchmark can score, at a fraction of the cost.
- Reuse every step after the LLM calls unchanged, so score differences come from extraction only.
- No behaviour change to `extract_metrics`.

**Non-Goals:**

- Matching `extract_metrics`' quality; the benchmark decides whether the loss is acceptable.
- English `desc`, `context` and `keywords` for Chinese documents, or detailed object nodes.
- Changing the shared LLM client, the coordinator's seed logic, the scorer's scoring or the gold
  rules.
- Deployment. This change makes the processor selectable; using it by default is a later decision.

## Decisions

### D1. A mode on `MetricsProcessor`, not a new type that embeds it

`NewMetricsSimpleProcessor` builds a `MetricsProcessor` with `NewMetricsProcessor`, then sets a
`name` field (`extract_metrics_simple`), a `simple` flag and the simple prompt/model fields.
`Name()` returns `name` when set. The two entry points branch on `simple` only where they call the
LLM: `extractMetricsFromChunksWithLLM` (sequential) and `ProcessChunk` / `FinalizeChunkBatch`
(chunk batch). Everything else (status, save, merge, logs, indexing, `PostProcessIndex`) is shared.

*Alternative:* a `MetricsSimpleProcessor` struct embedding `*MetricsProcessor` and overriding
`Name()`. Go has no virtual dispatch: the shared methods would still call the embedded `Name()`
and write `extract_metrics` into logs and statuses, and every entry point would have to be
duplicated to reach the new extraction step.

### D2. Share the post-LLM tail by extracting it, without behaviour change

The tail of `enrichMetricCandidates` (from `applyMetricFrames` to `judgeOpenValueRows`) becomes
`finishMetricRows(ctx, recordID, metrics, tagged, drops, chunks)`. `enrichMetricCandidates` calls it
as before; the simple path calls it with its own rows. Per-response steps that `extract_metrics`
runs per enrich batch (`dropRowsTaggedWithDropReason`) run per chunk response in the simple path.
`recoverMultiValueMetricRows` and `recoverMetricCandidateCoverage` are not called: there are no
candidates to cover, and unresolved multi-value rows keep the outcome a failed review has today.

### D3. Request layout: instructions as the system message

The simple path calls the extractor with `DocumentFirst = false`. The system message is the prompt
file's text, which contains the output schema; nothing per call is appended to it. The user message
is `canonicalChunkInputText(chunk.Lines, docCtx)`, the same serialization other processors use.
The schema lives in the prompt file, not in Go (ChenWeb rule: no prompts in code), so one file
defines the whole instruction prefix.

*Trade-off:* this processor's calls do not reuse the chunk prefix that other processors warm, but
the chunk is about 1k tokens against about 8k of instructions, so caching the instructions matters
far more.

### D4. Warm-up gate inside the processor

A small gate per run: the first call to arrive becomes the leader and runs alone; the others block
on a channel. When the leader returns, the gate reads its usage (`cacheTokenCounts` on this
processor's own client, the only call in flight then). It releases at once when at least half the
input tokens were cache hits (the prefix was already warm, the normal steady state), otherwise at
60 s after the leader started. `HandleEvent` and `InitChunkBatch` create a fresh gate. Waiting
respects context cancellation.

*Alternative:* lengthen `LLM_CALL_STAGGER`. It would delay every processor in every run, by a
fixed amount even when the cache is warm, and it applies only to the seed processor.

*Trade-off:* a cold run of a small document waits up to 60 s more. In chunk-batch mode, calls
waiting at the gate can hold coordinator slots for that time while other processors share the run.

### D5. Retry a failed chunk as two halves

When a chunk's call fails and the chunk has at least two normal lines, the chunk is re-sent as two
calls. Each extracts from one half of the normal lines; the other half is marked overlap (`o`), so
the model still sees the whole context but writes rows only for its half. This covers both
transport failures and oversized responses. A chunk that is one table line cannot be split and
fails as a candidate-pass chunk fails today (skipped with a warning in chunk-batch mode, error in
sequential mode).

### D6. Compact output and expansion in code

Response `{"lang", "m": [...], "x": [...]}` with the short keys of spec `metric-simple-extraction`.
Enum values (`upper_bound`, `requirement`, …) keep their full words: they are one or two tokens and
match the rules the prompt quotes from `extract_metrics`. Empty values are left out.
`expandSimpleMetricRow` maps a compact row to the long-named map that `normalizeMetricList` and
the shared tail expect, then fills the code-built fields (D7). A new structured-output contract
requires `m`.

There is no `frame` key. The model writes resources, methods and stages as labelled condition
parts (`试验装置：5等量块`, `药物：50％葡萄糖`, `给药途径：静脉推注`), which `applyMetricFrames` already
moves into `frame` for every row (`metricConditionLabelFacets` covers apparatus, medium, reagent,
sample, material, agent, method and stage labels in both languages). This drops the verbose nested
`frame` JSON with its own English fields. Lost: the `device` role (stays a condition part) and
`frame.procedure.step`, which only feeds the provision link; neither is scored.

`value_min` / `value_max` come from the notation step for `a~b`, `a-b` and `a±b`; the model gives
`lo` / `hi` only for a range written otherwise (a data format `n4..5,1`, occurrences `0..4`).

A quantity whose value is left open is listed in `x` with reason `no_value` instead of as a full
row: the shared `no_value` stage would set it aside anyway, and the full row costs output.

*Alternative:* positional arrays or tab-separated lines would save the remaining key overhead but
make nested `vs` / `f` awkward and errors harder to see. JSON with one- or two-letter keys keeps
most of the saving.

### D7. Fields built in code

As listed in the spec. Sources: the chunk's lines (`context`, `threshold_or_target`, section,
`location_type`), the row (`value_data_type`, `keywords`), fixed maps (`value_class_en` =
`value_class`, `unit_en` from a table of common Chinese units such as 个 → pcs, 次 → times,
倍 → times, 位 → digits, 天 → d, 小时 → h, 分钟 → min, 秒 → s), and constants (`confidence` 0.8,
`is_explicit_metric` true). `candidate_id` = `<chunk seq>_<n>` keeps the lineage key the logs and
`kb.metrics_dropped` use.

### D8. One merged prompt

`prompts/prompt-extract-metric-simple-v1.md` is written from candidates v30 and enrich v29. It
keeps every rule that decides rows (what is a metric, set-aside reasons, row granularity and value
structures, conditions and their resource/method labels, tables, data dictionaries, definitions,
counts, resource specs, category labels, scoring tables, value notation and enums) and drops what
explains past runs,
repeated examples and per-candidate bookkeeping. Target at most about 30 KB. Overlap lines: rows
only for normal lines (`flag` `n`), as Pass 1 instructs today; code also discards overlap-only rows.

### D9. Registration

- `productionProcessorSpecs`: `extract_metrics_simple` after `extract_metrics`, Phase B, depends on
  chunking, produces `metrics`. It is then selectable by name like any other processor.
- `ControlService` refuses a run whose selected processors include both metric processors
  (`metricProcessorConflict`, right after `selectProcessors`). The check is per run, not in
  `BuildProductionProcessorPlanFromFacts`: the runtime builds one plan from every configured
  processor, so a check there would stop the server from starting with both configured.
- `chunk_batch.go`: listed with the processors that must not be the cache seed; its calls do not
  plant the chunk prefix other processors share.
- Name lists in `control.go` (`requiresChunkingDependency`), `observability.go`
  (`processorRequiresLLM`) and `phase_d.go` (`normalize_assertions` waits on either metric
  processor's Phase C step).
- `runtime.go`: env keys for the runtime-config map; `applyMetricsRuntimeOverrides` accepts
  `EXTRACT_METRICS_SIMPLE_PROMPT` / `EXTRACT_METRICS_SIMPLE_MODEL_NAME`; the services map is
  keyed by `m.Name()` so the two processors do not overwrite each other.
- `config.local.toml` (not tracked): add to `[doc-processing].required_processors`, not to
  `default_processors`.
- `score_io.py` (`.agents` repo): `fetch_extraction_logs` accepts both processor names.

## Risks / Trade-offs

- [Lower recall: the two-pass design was introduced because one pass gave unstable counts on
  smaller models] → The benchmark measures it. If recall drops too far, the next step is two
  compact passes (keep D3 and D6), not tuning this one in place.
- [Rules lost while condensing the prompt; each rule came from a benchmark failure] → Keep every
  rule and its one deciding example; run the benchmark on the records whose failures produced the
  rules (416, 753, 371, 364, 376).
- [The warm-up adds up to 60 s on a cold run] → Skipped when the first call shows a warm cache.
- [Large single-call responses for dense table chunks] → The same volume as Pass 1 today
  (about 126 vs 130 tokens per row); the halves retry covers multi-line chunks.
- [English view and search of Chinese documents get less text; object nodes are subjects only]
  → Not scored; acceptable for the comparison, revisit before deployment.
- [The two processors overwrite each other's rows for a record] → Plans refuse both at once; the
  comparison runs them one after the other with `force_clear`, and `prompt_name` shows which wrote
  the rows.

## Migration Plan

No schema or data migration. Deploy the code, add the processor to `required_processors`, run it
on benchmark records with `force_clear`. Rollback: remove it from `required_processors`; re-run
`extract_metrics` on any record it wrote.

## Open Questions

- The 60 s warm-up wait and the 0.5 hit-ratio threshold come from one measurement (2026-08-16);
  check them on the first benchmark runs (`llm_usage_event`, first run versus second).
