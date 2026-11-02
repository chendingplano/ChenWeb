## 1. Database

- [x] 1.1 Goose migration: `kb_inputs_processing_mode_check` accepts `auto_offpeak` (Down reverts)

## 2. Peak hours (adjusted days)

- [x] 2.1 `isHoliday` matches only `day_kind = 'holiday'`; add `isAdjustedWorkday`
- [x] 2.2 `EvaluateActive`: adjusted day matches `workdays` and is not excluded by `weekends`
- [x] 2.3 Tests for the adjusted-day scenarios

## 3. Upload API

- [x] 3.1 `allowedProcessingModes` accepts `auto_offpeak` (upload + pending files)

## 4. Doc-processor off-peak gate

- [x] 4.1 `OffPeakGate` (window name / lead / poll from env, cached look-ahead evaluation, fail-open)
- [x] 4.2 Processing-mode lookup (`processingModeLoader` on `DocMetadataSQLStore`)
- [x] 4.3 Pipeline slot lease on context; release while held, re-acquire on resume
- [x] 4.4 Hold in `runSingleProcessorCollect` (except `blocking`) and before Phase C; held
      status/progress; stop cancels a hold
- [x] 4.5 Wire gate in `runtime.go`; unit tests for gate, lease and hold

## 5. Frontend

- [x] 5.1 "Auto - off-peak only" option, default, in both Auto Process pulldowns; `kbService.ts` type/default

## 6. Verify and document

- [x] 6.1 `go build`, `go vet`, `go test` for touched packages; `svelte-check` on touched files
- [x] 6.2 Update peak-hours devdoc (adjusted days, consumer) and write devdoc for this feature
