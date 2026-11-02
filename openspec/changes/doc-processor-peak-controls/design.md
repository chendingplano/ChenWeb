## Context

`ControlService.handleEvent` (`server/api/doc-processing/control.go`) runs one doc-processor
task: user-id check, routing (facets, the tier-3 classifier — an LLM call — and plan binding),
run-row creation, Phase A, Phase B (concurrent or chunk-batched), then Phase C post-processing.
Each task holds one of `MAX_DOC_PROCESS_PIPELINES` slots through a `pipelineSlotLease` on the
context. A user stop sets a flag in `kb.inputs.status`; `pollForStop` cancels the task context
with cause `ErrPipelineStopped`, which every processor already honours (`isCtxStopped`,
`CheckAndHandleStop`, context-aware HTTP requests).

`offpeak-only-auto-processing` added `OffPeakGate`: for `auto_offpeak` inputs it held each
LLM-using processor (and Phase C) while the `deepseek peak hours` record was active within a
10-minute look-ahead. Every provider request made by doc-processing goes through the shared
`OpenAIJSONClient` (`ExtractText`/`ExtractJSON`/`ExtractStructuredJSON` → `extractTextWithFormat`,
`Embed`, `EmbedBatch`) or a `llm.Client` built with `NewClient` (openai, anthropic, jev,
jev_emulated). The doc-review subscriber runs in the same process and shares the model permits.

## Goals / Non-Goals

**Goals:**
- One engine setting decides when doc-processor tasks run; a task's own mode overrides it.
- Configurable behaviour for active tasks that cross a peak boundary: `ignore`, `pause-llm`, `abort`.
- No provider request is sent from a held task during its disallowed span under `pause-llm`.
- Every wait/start/hold/resume is in the doc-processor log; each LLM hold and its end is a row in
  `llm_usage_event` that usage reports do not count as a request.

**Non-Goals:**
- `pause-exec` (pausing arbitrary execution). Rejected at startup.
- Configuring a peak mode for auto-launched tasks (parser → line files → doc processors). The
  resolution takes a task mode, so a later source can supply one.
- Interrupting a provider request that was already sent.
- Surviving a doc-processor restart while waiting or held (in memory, as before).
- Other task engines (pdf-parser, doc-service).

## Decisions

1. **Config shape.** `[[task-engines.doc-processor]]` (array of tables, at most one entry) read
   from the merged app config via `config.GetDocProcessorTaskEngine()`. Unknown or unsupported
   values (including `pause-exec`) and a second entry fail doc-processor startup, so a typo never
   silently disables peak control.

2. **Mode precedence.** `effective = taskMode if taskMode != "" else engine.default_mode`.
   `kb.inputs.processing_mode = 'auto_offpeak'` maps to task mode `auto_offpeak`; any other value
   maps to no task mode. A failed lookup is logged and treated as no task mode. Alternative —
   keeping the old per-document gate beside the engine — gives two overlapping hold mechanisms
   and makes `ignore` not mean ignore; rejected.

3. **Peak evaluation.** The same named Peak Hours record (`DOC_PROCESS_OFFPEAK_PEAK_HOURS_NAME`,
   default `deepseek peak hours`) evaluated at the current minute — no look-ahead — cached for
   `DOC_PROCESS_OFFPEAK_POLL_SEC` and shared by all waiters (`PeakWindow`). Admission and boundary
   detection use the same instant, so "permitted" has one meaning. An evaluation error means
   "permitted" for every mode (WARN): failing closed would stall all work, for `auto_peak` too.

4. **Admission before routing.** The wait happens right after the user-id check, before routing,
   because routing already calls an LLM. The stop poller and stale-stop-flag clear move ahead of
   it so Stop works while waiting. While waiting the task suspends its slot lease, and it
   re-checks the span after re-acquiring a slot (a slot wait can cross a boundary). The
   `doc_processing` status entry shows `running` with `progress` "waiting for off-peak hours
   (<name>)" / "waiting for peak hours (<name>)"; no processor name, so stuck-pipeline auto-heal
   leaves it alone. A stop while waiting writes `stopped` and creates no run row.

5. **Boundary rules apply only to tasks with mode ≠ `none`**, after admission:
   - `ignore`: nothing.
   - `abort`: a watcher re-checks every poll interval and cancels the task context with
     `errPipelineAborted`, which wraps `ErrPipelineStopped` so every existing stop path stops
     the work, including routing and Phase C. The task ends `stopped` (run row and status entry
     carry "aborted at peak-hour boundary"); being final, it is never resumed. A new status value
     was rejected: it would need UI and status-parser changes for no behavioural gain.
   - `pause-llm`: the task context carries an `llm.RequestGate`. Held calls keep the pipeline slot:
     the task is active, and no new task is admitted during the same span anyway.

6. **Shared request gate.** `llm.RequestGate{Wait(ctx, call) error; Open(ctx) bool}` installed
   with `llm.WithRequestGate`. Every shared client calls it immediately before each provider
   request (one HTTP attempt = one gate pass, so structured-output retries are gated too).
   `OpenAIJSONClient` waits before its rate limiter and model permit, then calls `Open` once the
   permit is held; if the span closed meanwhile it releases the permit and waits again, so a held
   call never sits on a permit that the doc-review subscriber needs. Rejected alternatives: gating
   inside each processor (25+ call sites, easy to miss one); an `http.RoundTripper` wrapper (runs
   after the permit is taken and has no call identity).

7. **Lifecycle events.** The doc-processing gate writes through the installed usage sink with
   `EventKind` `llm_hold` (when a call starts waiting), then `llm_resume` (released) or
   `llm_hold_cancelled` (task stopped/aborted while held). Identity comes from the call
   (`RequestGateCall.Usage`: account, profile, provider, base URL, model, prompt, call reason/loc),
   with user/run/record ids filled from the task context when the call lacks them. Metadata:
   `hold_id` (pairs the rows), `peak_mode`, `cross_peak_mode`, `peak_hours_name`, `call_kind`,
   and `held_ms` on the closing row. Tokens are 0. A write failure is logged at WARN and never
   blocks or fails the call — the same best-effort rule as request capture.
   `llm_usage_event.event_kind TEXT NOT NULL DEFAULT 'request'` with a CHECK constraint; the
   model-activity report, today summary and daily account report filter `event_kind = 'request'`.
   Event lists return the kind. Rejected: metadata-only tagging, which needs JSON predicates in
   every aggregate and leaves untagged rows ambiguous.

8. **Engine log.** The doc-processor runtime logger (`CWB_DOCPROC_001`, the process's own log file
   and stdout) is the engine log. All task and call hold/resume entries go there with
   `record_id`, `run_id`, `peak_mode`, `cross_peak_mode` and, for calls, `hold_id`, model and
   call location.

9. **Old gate removed.** `holdForOffPeak` (in `runSingleProcessorCollect`, before Phase C and in
   the chunk-batch path), `processorUsesLLM`, the look-ahead and its env var go. The window
   evaluator, `GetProcessingMode` and the slot lease stay.

## Risks / Trade-offs

- [Plain `auto` uploads now follow the engine default] → the default `none` keeps today's
  behaviour; the devdoc states it plainly for operators who set `auto_offpeak`.
- [Without the look-ahead, a task admitted at 08:59 under `ignore` runs into peak] → choose
  `pause-llm` or `abort` where that matters.
- [`pause-llm` holds a call wrapped in a caller timeout] → the call fails with its deadline.
  No doc-processor task path wraps provider calls in one today (`embedQueryText`'s timeout is
  reached only from doc-reviews and kbhandler, which carry no gate).
- [Abort latency up to one poll interval; an in-flight request is cancelled with the context] →
  acceptable for a cost control; tokens of that one request may still be billed.
- [Waiting/held tasks are lost on restart and their status stays `running`] → unchanged from the
  previous gate; Stop and re-run recovers them.
- [Two extra rows per held call] → bounded by concurrent in-flight calls (model permits).

## Migration Plan

1. Apply the `event_kind` migration (default fills existing rows with `request`).
2. Deploy shared + ChenWeb together; with no config block the engine is `none`/`ignore`, and the
   only change is that `auto_offpeak` uploads wait at admission instead of between processors.
3. Rollback: revert the binaries; the column can stay (default `request`).

## Open Questions

- Auto-launched task peak modes (deferred by the user, 2026-11-02).
