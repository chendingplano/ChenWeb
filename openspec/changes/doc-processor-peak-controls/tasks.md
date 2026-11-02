## 1. Shared library (`shared/go/api/llm`)

- [x] 1.1 `RequestGate` interface, `RequestGateCall`, `WithRequestGate`, wait/open helpers; tests
- [x] 1.2 Gate every provider request: `OpenAIJSONClient` chat (before rate limit/permit, re-check
      after permit, release and re-wait if closed), `Embed`, `EmbedBatch`; `openai.go`,
      `anthropic.go`, `jev.go`, `jev_emulated.go` `Complete`/`Stream` before send; tests
- [x] 1.3 `EventKind` on `UsageCaptureInput` / `UsageCaptureRecord`
- [x] 1.4 `go test ./...` in `shared/go` (no dependency change, so `go work sync` not needed); `tax` builds

## 2. Usage events (ChenWeb)

- [x] 2.1 Goose migration: `llm_usage_event.event_kind` (default `request`, CHECK)
- [x] 2.2 Sink writes `event_kind`; tests
- [x] 2.3 Aggregates filter `event_kind = 'request'`; lists return `event_kind`; tests
- [x] 2.4 Show the event kind in the LLM usage event list (en + zh-cn)

## 3. Engine policy (ChenWeb doc-processing)

- [x] 3.1 Config parse/validate `[[task-engines.doc-processor]]`; document defaults in `config.toml`
- [x] 3.2 `PeakWindow` (cached current-minute evaluation, fail-open) replacing `OffPeakGate`;
      remove `holdForOffPeak`, `processorUsesLLM`, look-ahead
- [x] 3.3 Mode resolution (task mode → engine default) and permitted check
- [x] 3.4 Admission in `handleEvent`: stop poller first, waiting status/progress, slot release,
      re-check after re-acquire, stop while waiting, logs
- [x] 3.5 `abort` watcher with `errPipelineAborted`; status/run-row error
- [x] 3.6 `pause-llm` request gate: hold/resume/cancel logs and `llm_usage_event` lifecycle rows,
      attribution from task context, write failures logged only
- [x] 3.7 `runtime.go` wiring; remove the temporary `pause-llm` startup rejection
- [x] 3.8 Tests: admission both directions, `none`, stop while waiting, slot release/re-check,
      eval error, abort, pause-llm hold/resume/cancel with a real `OpenAIJSONClient` against a
      test server, event write failure, concurrent tasks

## 4. Verify and document

- [x] 4.1 `go build`, `go vet`, `go test` for touched ChenWeb packages; `bun run check`
- [x] 4.2 Startup path check with the local config override
- [x] 4.3 Update devdoc `2026092802-devdoc-offpeak-only-auto-processing.md`
- [x] 4.4 Commit shared and ChenWeb separately through `jj`, preserving unrelated changes

Verification notes (2026-11-02): all touched packages pass except the unrelated
`TestKBMetricsReadersExcludeResolvedRows` guard (flags `kbhandler/question_generator_worker.go`).
The startup path was checked with a throwaway test through `config.LoadConfig` (local override →
`auto_offpeak`/`ignore`); the migration was applied to `miner` by the live server and Down→Up
checked in a rolled-back transaction; the real `deepseek peak hours` record evaluated as expected.
Not verified: a live doc-processor run across a real peak boundary, lifecycle rows written by a
live run, and the event-kind labels in a logged-in browser.
