## Why

The Doc Processor Manual Launch panel currently re-runs all selected records every time regardless of their processing state, and offers no upper bound on how many records are dispatched per launch. Operators need fine-grained control over which records qualify for a run and a safety cap on throughput.

## What Changes

- Add a **Run Mode** radio-button group (4 options) that filters which records from the selection are actually dispatched when Launch is clicked.
- Add a **Max Records to Run** numeric input that caps the number of records dispatched in a single launch.
- Change the `force` flag in the dispatched event payload to reflect the selected run mode (`true` only for Force Run mode).
- The existing "Select Failed" / "Select Incompleted" buttons (which control *which processors* are checked) remain unchanged.

## Capabilities

### New Capabilities

- `doc-processor-run-mode`: A run-mode radio group and max-records input in the Manual Launch panel that control record filtering and dispatch volume before launch.

### Modified Capabilities

<!-- none -->

## Impact

- **Frontend only** — all logic lives in `doc-processor-dashboard-view.svelte`.
- `confirmLaunch()` / `doLaunch()` gain run-mode filtering and max-records slicing before iterating over selected record IDs.
- The `force` field in the `kb.line-file-generated` event payload becomes `true` only when run mode is `force`; backend is unchanged.
- No API, schema, or backend changes required.
