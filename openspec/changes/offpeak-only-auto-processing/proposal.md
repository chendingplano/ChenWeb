## Why

Most LLM work in the doc-processor runs on DeepSeek, which charges more and is less available
during its peak window (see `2026092402-devdoc-peak-hours-admin.md`). Uploads today start
processing immediately, whatever the time. An upload mode that defers LLM work to off-peak hours
cuts cost without anyone having to time their uploads by hand.

## What Changes

- New upload processing mode `auto_offpeak`, labelled **"Auto - off-peak only"**, in the
  Auto Process pulldown of *Knowledge → File Management → Upload Files* (and the Pending Files
  dialog). It becomes the default selection in both.
- The doc-processor holds each LLM-using processor of an `auto_offpeak` document while the
  configured peak-hours window is active **or starts within a lead time** (default 10 min), and
  resumes it automatically once the window is over. A held pipeline gives up its pipeline slot,
  so plain "Auto" documents are not starved.
- `kb.inputs.processing_mode` CHECK constraint accepts `auto_offpeak`; the upload / pending-files
  APIs accept it; zip children inherit it (existing behaviour for non-`auto` modes).
- Fix: peak-hours evaluation treated adjusted working days (`calendar_holidays.day_kind =
  'adjusted'`) as holidays, so e.g. CN Sunday 2026-01-04 counted as off-peak. Adjusted days
  now count as working days.

## Capabilities

### New Capabilities
- `offpeak-doc-processing`: the `auto_offpeak` processing mode and the doc-processor's
  hold/resume behaviour around peak hours.

### Modified Capabilities
- `peak-hours-admin`: evaluation honours adjusted working days (holiday exclusion only matches
  `day_kind = 'holiday'`; an adjusted day is a workday and is not a weekend).

## Impact

- DB: migration updating `kb_inputs_processing_mode_check`.
- Go: `server/api/kbhandler/upload_handler.go` (allowed modes),
  `server/api/doc-processing/` (new off-peak gate, wiring in `control.go` / `runtime.go`,
  processing-mode lookup), `server/api/peakhourshandler/` (adjusted days, exported lookup).
- Web: `kb-import-view.svelte`, `kbService.ts`.
- Python pdf-parser: none (it already treats every mode except `pdf_parsing` as auto).
- Env (doc-processor): `DOC_PROCESS_OFFPEAK_PEAK_HOURS_NAME` (default `deepseek peak hours`),
  `DOC_PROCESS_OFFPEAK_LEAD_MINUTES` (default 10), `DOC_PROCESS_OFFPEAK_POLL_SEC` (default 30).
