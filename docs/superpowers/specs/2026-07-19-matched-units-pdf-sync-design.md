# Matched-Units PDF Sync Design

## Goal

In the Document Review Report's Finding Details panel, the "Doc Review Log" block has a "View matched_units" button that opens a dialog listing the artifacts `kb.doc_review_logs.matched_units` matched against the finding's artifact (e.g. `430_mtc_1`, `415_mtc_1`). Clicking a matched-unit row already drills into a detail view (its fields plus a source-context line table) inside the dialog. This change makes that same click also switch the right-hand PDF/Document Structure panel to that unit's source document, jump to the correct page, and highlight its source lines — mirroring what already happens when a finding card in the left panel is clicked.

## Scope

- Applies only to rows inside the "View matched_units" dialog (`matched-units-dialog.svelte`), reached from the Finding Details panel on `doc-review-report/[id]`.
- Does not change the "View findings" or "View detail" dialogs, which show findings/LLM-call JSON without a per-row document location to jump to.
- No database or API changes: `source_record_id`, the nested artifact's `source_line_spans`, and `source_context` are already present in the stored `matched_units` JSON (`review-metrics.go`, `review-provisions.go`, `review-inventory-items.go`; `review-entities.go` carries `source_record_id` only, no line data).

## Data already available

Each `matched_units[i]` entry is shaped `{ <artifact-key>: {...}, source_record_id, source_filename, source_doc_authority, match_via, match_rank, source_context }`, where `<artifact-key>` is `metric` / `provision` / `item` / `entity` depending on which reviewer produced it. The nested artifact object carries `source_line_spans` (e.g. `["27"]`, `["53-56"]`) — the artifact's own line span(s) in its source document, in the same `"N"` / `"N-M"` / `"N:M"` format used server-side (`parseArtifactSpan`, `review-artifact-window.go:62`). `source_context` (`[{line_number, content}]`) is a fallback when spans are absent (true today for `entities`).

## Changes

### 1. `doc-review-json-dialog.ts`
Add `matchedUnitFocusTarget(unit: unknown): { recordId: number; lineNumbers: number[] } | null`:
- Returns `null` if `unit.source_record_id` is not a positive number.
- Finds the nested artifact object generically (the first record-valued field on `unit` that isn't one of the known sibling fields: `source_record_id`, `source_filename`, `source_doc_authority`, `match_via`, `match_rank`, `confidence`, `source_context`) — this avoids hardcoding `metric`/`provision`/`item`/`entity` per call site.
- `lineNumbers`: expands `source_line_spans` (each entry parsed as `"N"` / `"N-M"` / `"N:M"`, mirroring `parseArtifactSpan`) if present and non-empty; otherwise falls back to the `line_number` values in `source_context`; otherwise `[]`.
- Returns `{ recordId, lineNumbers }` even when `lineNumbers` is `[]` (still enables switching to the right document/page 1, just without a line highlight).

### 2. `matched-units-dialog.svelte`
- New optional prop `onFocusUnit?: (recordId: number, lineNumbers: number[]) => void`.
- In the list-row `onclick` handler (currently `() => (selected = i)`), also compute `matchedUnitFocusTarget(unit)` and, if non-null, call `onFocusUnit(recordId, lineNumbers)`. This fires alongside opening the detail view — one click, both effects (per user decision).

### 3. `finding-details-panel.svelte`
- New optional prop `onFocusMatchedUnit?: (recordId: number, lineNumbers: number[]) => void`, passed straight through to `<MatchedUnitsDialog onFocusUnit={onFocusMatchedUnit} .../>`.

### 4. `doc-review-report/[id]/+page.svelte`
- Passes `onFocusMatchedUnit={(recordId, lineNumbers) => void structureView?.focusExternalArtifact(recordId, lineNumbers)}` to `<FindingDetailsPanel>`.

### 5. `doc-structure-view.svelte`
- New exported `async function focusExternalArtifact(recordId: number, lineNumbers: number[])`:
  - If `currentInput?.id !== recordId`, `await loadStructureForRecord(recordId)` (existing function; resets page/selection/highlight and loads the new record's lines + PDF).
  - If `lineNumbers.length > 0`, `await focusSourceLines(lineNumbers)` against the newly loaded lines.
- Safety fix to the existing `focusSourceLines` (used by the left panel's normal finding clicks, via `+page.svelte`'s `onFocusFinding`): at its start, if `lockedRecordId != null` and `currentInput?.id !== lockedRecordId`, `await loadStructureForRecord(lockedRecordId)` first. Without this, clicking a normal finding after having viewed a matched unit's external document would silently no-op (searching for line numbers among the wrong document's loaded lines).
- New derived `viewingExternalRecord = lockedRecordId != null && currentInput != null && currentInput.id !== lockedRecordId`.
- New `function backToLockedRecord()`: if `lockedRecordId != null`, `void loadStructureForRecord(lockedRecordId)`.
- Render placement: a thin banner bar at the top of the right-hand Document Structure panel (the panel already hosting the PDF viewer and its page/zoom toolbar), directly above the PDF page area, shown only when `viewingExternalRecord` is true. Content: the external document's title/filename and a "← Back to reviewed document" button calling `backToLockedRecord()`.

## Error handling

- If a matched unit has no valid `source_record_id`, `onFocusUnit` is never invoked — the row still opens its existing detail view, just without a PDF jump.
- If `loadStructureForRecord` fails (e.g. record deleted or fetch error), the existing `errorMsg` state in `doc-structure-view.svelte` surfaces it inline in the right panel; no crash, no dialog disruption.
- If `lineNumbers` resolves to `[]` (e.g. `entities` matches, which carry no line data), the panel still switches to the matched document at page 1, without a highlight.

## Testing and verification

- Add unit tests in the existing `doc-review-json-dialog.test.ts` for `matchedUnitFocusTarget`: span parsing (`"27"`, `"53-56"`, `"53:56"`), multiple spans, `source_context` fallback when spans are absent, `null` result when `source_record_id` is missing/invalid, and correct behavior across differently-keyed artifact types (`metric`/`provision`/`item`/`entity`).
- Manual verification via `mise dev`: open a report with metrics findings that have cross-document `matched_units`, open "View matched_units", click a unit, confirm the right panel switches to the correct document/page and highlights the expected line(s); confirm the back banner appears and returns to the reviewed document; confirm clicking a normal finding afterward still correctly re-locks to the reviewed document.

## Documentation impact

- This spec is the record of the new interaction; no other docs describe matched-unit dialog behavior today, so nothing else needs updating.
