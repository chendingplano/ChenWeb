## 1. Reactive State

- [x] 1.1 Add `runMode` state variable typed as `'unfinished' | 'failed' | 'unfinished_failed' | 'force'` defaulting to `'unfinished_failed'` in `doc-processor-dashboard-view.svelte`
- [x] 1.2 Add `maxRecords` state variable typed as `number` defaulting to `5`

## 2. Filtering Logic

- [x] 2.1 Add local helper `filterByRunMode(records: KbInputRecord[], mode: RunMode): KbInputRecord[]` that uses `computeStages(rec)` and returns only qualifying records per the mode rules (pending/in-progress for `unfinished`; failed for `failed`; all three for `unfinished_failed`; pass-through for `force`)
- [x] 2.2 In `confirmLaunch()`, replace `selectedRecords.map(...)` with `filterByRunMode(selectedRecords, runMode).slice(0, maxRecords).map(...)` and update the success toast count to reflect the actual dispatched count
- [x] 2.3 In `doLaunch()`, change the hardcoded `force: true` in the `kb.line-file-generated` payload to `force: runMode === 'force'`

## 3. UI — Run Mode Radio Group

- [x] 3.1 Add a new row above the existing "Select all / Select Failed / Select Incompleted / Launch" row in the Manual Launch HTML section (around line 1362)
- [x] 3.2 Render four radio inputs bound to `runMode` with labels: "Run Unfinished Only" (`unfinished`), "Run Failed Only" (`failed`), "Run Unfinished & Failed" (`unfinished_failed`), "Force Run" (`force`) — style consistently with the panel's dark theme using inline styles matching `textSecondary` / `accent` / `surface2`

## 4. UI — Max Records Input

- [x] 4.1 Add a "Max Records to Run" numeric input bound to `maxRecords` in the same new row as the radio group (or inline to its right), styled to match the panel
- [x] 4.2 Disable the Launch button when `maxRecords < 1` or `isNaN(maxRecords)` (extend the existing `!someProcessorsSelected()` disabled condition)
