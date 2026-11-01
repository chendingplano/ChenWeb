## Why

`extract_metrics` gives good benchmark scores but costs too much: record 800 (17 chunks) costs
¥1.40 and record 416 (10 chunks) ¥0.34 at DeepSeek off-peak prices (runs 240 and 237,
2026-10-19). Three things drive the cost:

- **Input that is never cached.** Both passes put the chunk text first and the 40 KB / 70 KB
  instructions after it, so the instructions cannot be shared across chunks or documents. Run 240
  had 800k cache-miss input tokens and 79k hits; input was 57% of its cost (79% on 416).
- **Two passes.** Every candidate is written out by Pass 1 and then read and rewritten by Pass 2,
  which uses about three calls per chunk (49 calls for 17 chunks on record 800).
- **Large output rows.** An enrich v29 row costs about 500 output tokens and its candidate about
  126 more. Of the enrich output, about 31% is repeated key names, 32% English translations, 20%
  the `objects` array and 11% fields that are almost always empty. The benchmark scorer reads none
  of these.

We want a cheaper processor to compare with `extract_metrics` on the metric benchmark, accepting
some loss of quality, and decide from the scores and costs whether to deploy it.

## What Changes

- New opt-in doc processor `extract_metrics_simple`. It writes the same tables as
  `extract_metrics` (`kb.metrics`, `kb.metrics_dropped`, logs, the `.metrics` file) and is never
  added to a default pipeline.
- One LLM call per chunk, which both finds the metrics and returns the final rows. No candidate
  pass, no enrich batches.
- The constant instructions come first in the request and the chunk text second, so the
  instructions are a cached prefix for every chunk and document. The first chunk's call runs alone
  and the others wait until the prefix can be reused (or the first call shows it already was).
- One merged prompt, `prompts/prompt-extract-metric-simple-v1.md` (at most about 30 KB), written
  from candidates v30 and enrich v29. It keeps their rules and drops their history, rationale and
  repeated examples.
- Compact output: short keys, empty fields left out, English only for the name, subject and
  condition. The other stored fields are built in code from the source lines and the row.
- Items the model sets aside come back as a compact list (lines, name, reason) and are saved to
  `kb.metrics_dropped` like enrich-stage drops.
- Everything after the LLM call is shared with `extract_metrics`: frames, condition inheritance,
  dedup, value notation, every set-aside stage, saving, merge mode, Phase C indexing, provision
  links, the ontology harvest and reference resolution. The multi-value review call is not made;
  such rows stay `unresolved`.
- A plan that requests both `extract_metrics` and `extract_metrics_simple` is refused: they would
  write the same rows.

`extract_metrics` itself does not change behaviour. Its post-LLM tail moves into a shared function.

## Capabilities

### New Capabilities
- `metric-simple-extraction`: the `extract_metrics_simple` processor — one call per chunk, the
  request layout and warm-up, the compact output contract, the fields built in code, set-aside
  items, the steps shared with `extract_metrics`, and opt-in registration.

### Modified Capabilities

None. The existing metric specs (`metric-condition`, `metric-row-granularity`,
`metric-value-notation`, `metric-statement-kind`, …) describe what a metric row is, and the new
processor follows them through its prompt and the shared post-LLM steps.

## Impact

- **Code** (`server/api/doc-processing/`): new `extract-metrics-simple.go` and its tests;
  `extract-metrics.go` (processor name and mode fields, the shared post-LLM function, the
  chunk-batch hooks); registration in `runtime.go`, `processor_plan.go`, `processor_registry.go`
  and the processor lists in `control.go`, `observability.go`, `chunk_batch.go` and `phase_d.go`.
- **Prompt**: new `prompts/prompt-extract-metric-simple-v1.md`.
- **Configuration**: env `EXTRACT_METRICS_SIMPLE_PROMPT` and `EXTRACT_METRICS_SIMPLE_MODEL_NAME`
  (falls back to `ENRICH_METRICS_MODEL_NAME`, then `EXTRACT_METRICS_MODEL_NAME`); `config.local.toml` lists the processor in
  `[doc-processing].required_processors` so it can be selected, not in `default_processors`.
- **Data**: no schema change. Rows carry `prompt_name = prompt-extract-metric-simple-v1.md`, so a
  record's rows show which processor wrote them.
- **Benchmark**: unchanged. `score_io.py` and the in-app Benchmark page score whatever
  `kb.metrics` holds for a record, so the two processors are compared by running each with
  `force_clear` and scoring.
- **Not covered by the benchmark**: English text for Chinese documents (no `desc`, `desc_en` or
  `context_en`), object nodes (synthesized from `subject` only), and search text, which is built
  from fewer fields.
