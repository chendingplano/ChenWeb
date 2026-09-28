## Context

`kb.inputs.processing_mode` (`auto` | `upload_only` | `pdf_parsing`) is chosen at upload time.
The pdf-parser publishes the doc-processing event for every mode except `pdf_parsing`; the
doc-processor (`server/api/doc-processing`, `ControlService`) then runs Phase A processors
sequentially, Phase B concurrently, and Phase C post-processing. Every pipeline holds one of
`MAX_DOC_PROCESS_PIPELINES` slots (default 10). Peak windows are named `public.peak_hours`
records evaluated by `peakhourshandler.EvaluateActive`; the only record today is
`deepseek peak hours` (09:00–12:00, 14:00–18:00 Asia/Shanghai, CN workdays).

## Goals / Non-Goals

**Goals:**
- `auto_offpeak` documents never *start* an LLM-using processor during peak or within the lead
  time before it; they continue automatically after it.
- Held work is visible (processor status stays `active` with progress text) and stoppable
  (the existing Stop button cancels a held pipeline).
- Held pipelines do not block other documents' pipeline slots.

**Non-Goals:**
- Pausing a processor in the middle of its LLM calls. A processor that already started finishes
  even if peak begins; the lead time is the mitigation.
- Holding by model/provider. The hold applies to every LLM-using processor regardless of which
  model it is configured with.
- Surviving a doc-processor restart while held: a held pipeline is an in-memory goroutine, just
  like a running one, and restart recovery is unchanged.

## Decisions

1. **Gate at the processor boundary** (`runSingleProcessorCollect`, plus once before Phase C).
   There is no single LLM call chokepoint (`llmCallController` is used by one processor only),
   and a processor boundary is a clean resume point: nothing is half-done. Alternative — stop the
   pipeline and re-dispatch later — needs a scheduler and re-runs Phase A; rejected.
2. **Which processors are "LLM-using".** Everything except `blocking`. Even `static_analyzer`
   (TOC detection) and `chunking` call LLMs. Phase C is held as a whole, since some stages
   (object-ambiguity resolution) call LLMs.
3. **"About to enter"** = the window is active at any minute in `[now, now + lead]`, evaluated
   minute by minute with `EvaluateActive`. The result is cached for the poll interval and shared
   by all waiters, so the per-minute holiday lookups run at most once per poll.
4. **Configurable window name**, default `deepseek peak hours`. A missing record or evaluation
   error fails open (not held) with a WARN log, matching peak-hours' own fail-open holiday rule;
   holding forever on a misconfiguration would silently stall all uploads.
5. **Status while held**: processor entry `proc_status = active`, `progress = "held: waiting
   for off-peak hours (<name>)"`. Using a non-final status keeps `isStuckPipeline` from
   auto-healing a held pipeline; no new status value means no UI/status-parser changes.
6. **Slot release**: the pipeline's slot is wrapped in a lease carried on the context. The first
   waiter of a pipeline releases the slot; the last waiter to resume re-acquires it (blocking
   until one frees up) before running.
7. **Mode lookup** via an optional `processingModeLoader` interface on the input store, so the
   existing test fakes stay untouched. A lookup failure falls back to `auto` (no hold).
8. **Server default stays `auto`** when the form field is omitted, to keep other API callers'
   behaviour; only the UI default changes.
9. **Adjusted days fix**: `isHoliday` filters `day_kind = 'holiday'`; an adjusted date makes
   `workdays` match and `weekends` not exclude.

## Risks / Trade-offs

- A long processor started just before the lead window can run into peak → raise
  `DOC_PROCESS_OFFPEAK_LEAD_MINUTES` if that matters.
- Temporary slot over-subscription: while one Phase B goroutine is held (slot released), sibling
  non-held goroutines of the same pipeline keep running. They are bounded by the same pipeline,
  so the overshoot is small.
- Many held pipelines each keep a goroutine and a stop-poller (1 query/s) alive during peak.
  Acceptable at current upload volumes.
