## Context

The Doc Processor Manual Launch panel in `doc-processor-dashboard-view.svelte` lets operators pick KB input records and processors, then click Launch. Currently `confirmLaunch()` dispatches every selected record unconditionally with `force: true`. Operators need to (a) filter which records qualify for a run based on their processing state and (b) cap dispatch volume per session.

All filtering logic is pure frontend — `selectedRecords` already holds the hydrated `KbInputRecord[]` with status arrays, and `computeStages(rec): StageInfo[]` already maps those statuses to typed `StageStatus` values (`'pending' | 'in-progress' | 'success' | 'failed'`). No backend or API changes are needed.

## Goals / Non-Goals

**Goals:**
- Add a `runMode` reactive state variable (`'unfinished' | 'failed' | 'unfinished_failed' | 'force'`) defaulting to `'unfinished_failed'`.
- Add a `maxRecords` reactive state variable (positive integer, default `5`).
- Filter `selectedRecords` in `confirmLaunch()` based on `runMode` before iterating.
- Cap the filtered list to `maxRecords`.
- Pass `force: runMode === 'force'` instead of the hardcoded `force: true` in `doLaunch()`.
- Render a radio group and numeric input in the Manual Launch HTML section.

**Non-Goals:**
- Backend changes — the event payload shape and NATS routing are unchanged.
- Changing how "Select Failed" / "Select Incompleted" work (they control processor checkboxes, not record filtering).
- Persisting run mode or max-records across page reloads.
- Applying run-mode filtering to the per-record Restart flow.

## Decisions

### D1 — Filter in `confirmLaunch()`, not in `doLaunch()`

`doLaunch()` handles one record at a time and is also called by the Restart flow. Keeping run-mode logic in `confirmLaunch()` avoids polluting the single-record helper with bulk-launch concerns and leaves Restart unaffected.

Derive the eligible set:
```
const eligible = filterByRunMode(selectedRecords, runMode).slice(0, maxRecords);
```
Then iterate only over `eligible`.

### D2 — Helper `filterByRunMode(records, mode)` as a local function

A named function (not an inline ternary) keeps `confirmLaunch()` readable and is trivially testable if unit tests are added later. The function uses `computeStages(rec)` — already imported — to get typed stages, then checks `stage.status`:

| Mode | Qualifying stage statuses |
|---|---|
| `unfinished` | `'pending'` or `'in-progress'` |
| `failed` | `'failed'` |
| `unfinished_failed` | `'pending'`, `'in-progress'`, or `'failed'` |
| `force` | (no filter — return all) |

A record qualifies if **at least one** of its processor stages (excluding `staged`, `parsing`, `converting` which are managed by separate checkboxes) matches the condition.

### D3 — `force` event flag tied to run mode

Currently hardcoded to `true`. Change to `force: runMode === 'force'` so non-Force modes let the backend skip already-succeeded processors naturally.

### D4 — Radio group UI placement

Insert the run-mode radio group and max-records input between the "Select Incompleted" button and the Launch button row, as a new row inside the existing `flex items-center justify-between` container, or as a separate row above it. A dedicated row above is cleaner given four radio options.

### D5 — `maxRecords` validation: clamp on blur, ignore invalid keystrokes

Prevent submission with `maxRecords < 1` by disabling the Launch button when the value is invalid (same pattern as `!someProcessorsSelected()`). No toast needed — the disabled state is self-explanatory.

## Risks / Trade-offs

- **Stage scope for filtering**: The filter checks all processor stages on a record. If a record has a failed `parsing` stage but the operator has not checked `parse_file`, the record still qualifies under `failed` mode. This is intentional — run mode is about record eligibility, processor checkboxes are about what runs. Operators can combine both.
- **`maxRecords` = 5 default**: Prevents accidental bulk-runs. Operators who need more must explicitly increase it.
- **`force: false` behavior change**: Previously every manual launch sent `force: true`. With `unfinished_failed` (the new default), `force: false` is sent. The backend processor logic for `force: false` skips already-succeeded operations — this is the desired behavior and matches the filtering intent.
