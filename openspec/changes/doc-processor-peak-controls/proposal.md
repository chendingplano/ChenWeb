## Why

The off-peak support shipped in `offpeak-only-auto-processing` only holds the LLM processors of
documents uploaded as "Auto - off-peak only", and only between processors. Operators need one
engine-wide setting that decides when the doc-processor works (peak, off-peak, or any time), a
rule for what active work does when the time crosses a peak boundary, and an audit trail of every
hold and resume, including in `public.llm_usage_event`. Requirements:
`KnowledgeStore/doc-repo/devdocs/202609/2026092802-devdoc-offpeak-only-auto-processing.md`
and `KnowledgeStore/doc-repo/hand-offs/202611/2026110201-hand-offs-doc-processor-peak-controls.md`.

## What Changes

- New config block (merged `config.toml` / `config.local.toml`, at most one entry):
  `[[task-engines.doc-processor]]` with `default_mode` = `none` (default) | `auto_peak` |
  `auto_offpeak`, and `cross_peak_mode` = `ignore` (default) | `pause-llm` | `abort`.
  `pause-exec` and any other value fail startup.
- A task's **peak-hour mode** is the task's own mode when it has one, otherwise the engine's
  `default_mode`. Today the only source of a task mode is `kb.inputs.processing_mode =
  'auto_offpeak'`; every other processing mode leaves the task without one.
- **Admission**: a task whose mode does not permit the current time waits before any work
  (routing included), gives up its pipeline slot while waiting, shows a waiting note in its
  `doc_processing` status, can be stopped, and starts on its own when its permitted span returns.
- **Crossing a boundary** while active: `ignore` continues; `pause-llm` holds each new provider
  request (chat, structured output, embeddings, retries) until the span returns, while a request
  already sent finishes; `abort` cancels the task, which ends `stopped` with error
  "aborted at peak-hour boundary" and is not resumed.
- Every task wait/start and every LLM hold/resume is logged in the doc-processor log, and each
  LLM hold and its resume (or cancellation) is written to `llm_usage_event` as a lifecycle row.
- `llm_usage_event` gains `event_kind` (`request` default, `llm_hold`, `llm_resume`,
  `llm_hold_cancelled`). Usage aggregates (model activity, today summary, daily account report)
  count only `request` rows; event lists show the kind.
- **BREAKING (behavior)**: the per-processor off-peak gate is removed. An `auto_offpeak` document is
  now governed like any other task with mode `auto_offpeak`: it no longer waits between processors
  once admitted (that is now `cross_peak_mode`'s job), and the 10-minute look-ahead
  (`DOC_PROCESS_OFFPEAK_LEAD_MINUTES`) is retired. Plain `auto` uploads follow the engine default
  instead of always running immediately.
- Shared library: `llm.WithRequestGate` takes a `RequestGate` that every provider request in the
  shared clients passes before it is sent; `UsageCaptureInput`/`UsageCaptureRecord` gain
  `EventKind`.

## Capabilities

### New Capabilities
- `doc-processor-peak-controls`: engine-level peak-hour mode and task-mode precedence, admission
  waiting, cross-peak boundary rules, and hold/resume logging and lifecycle events.

### Modified Capabilities
- `offpeak-doc-processing`: `auto_offpeak` becomes the task's peak-hour mode under the engine
  controls; the per-processor hold with look-ahead and the held-processor slot release are
  removed (superseded by admission and boundary rules).

## Impact

- ChenWeb Go: `server/cmd/config/task_engine.go`; `server/api/doc-processing/`
  (`task_engine_policy.go`, `offpeak_gate.go`, `control.go`, `chunk_batch_coordinator.go`,
  `runtime.go`); `server/api/llmusage/sink.go`; `server/api/llmreporthandler/store.go`.
- DB: goose migration adding `llm_usage_event.event_kind` with a CHECK constraint.
- Shared Go (`shared/go/api/llm`): `request_gate.go`, `openai_client.go`, `openai.go`,
  `anthropic.go`, `jev.go`, `jev_emulated.go`, `usage_capture.go`. Additive except
  `WithRequestGate`'s parameter type (no callers yet).
- Web: event kind shown in the LLM usage event list (`en.json` / `zh-cn.json`).
- Config: `config.toml` documents the block with defaults.
- Docs: devdoc `2026092802-devdoc-offpeak-only-auto-processing.md`.
