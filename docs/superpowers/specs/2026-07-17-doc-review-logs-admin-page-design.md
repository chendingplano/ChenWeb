# Doc Review Logs Admin Page Design

## Goal

Add a read-only admin page for `kb.doc_review_logs` at **SYSTEM ADMIN → Logs → Doc Review Logs**. It lets administrators inspect individual document-review execution records without exposing large JSON values in the table.

## Scope

The change adds a paginated, filterable API and a matching Svelte view in the existing home3 content panel. No database migration or mutation actions are required.

## Navigation and page

- Add `Doc Review Logs` under the existing `SYSTEM ADMIN → Logs` menu.
- The page follows the existing Doc Processor Logs and LLM Usage Logs visual and interaction patterns.
- Keep refresh, filtering, pagination, loading, and error states.
- Columns, in order: Input Record ID, Run ID, Unit Key, Aspect, Unit Type, Unit Location, Outcome, Pass, # Findings, Detail, Create Time.

## API and data flow

- Add an admin GET endpoint that reads `kb.doc_review_logs` with server-side pagination and filters matching the page controls.
- Return scalar table fields plus the JSON fields needed for the dialogs: `unit_location`, `matched_units`, `findings`, and `detail`.
- Order results newest first by `create_time` (with a stable ID tie-breaker).
- The handler validates pagination and filters, then delegates SQL construction and scanning to a focused store function. It returns the row slice and total count.
- The query uses only bound parameters for values and an allowlist for any selectable ordering values, consistent with existing log stores.

## Table behavior

- `# Findings` is the size of `findings` when it is a JSON array; null, non-array, or empty values render as `0`.
- `UNIT LOCATION` is a compact human-readable rendering of its JSON value. The complete value remains available in Detail.
- `Detail` opens a modal containing the row JSON values rendered recursively as name-value entries. Arrays display indexed entries; scalar values display directly; null displays explicitly.

## Artifact interaction

- Treat a Unit Key as an artifact ID only if it exactly matches `<record_id>_(mtc|prv|inv)_<seqno>`.
- Only matching keys are interactive. Selecting one loads the artifact by that ID through the project’s existing artifact retrieval path and displays it in the same recursive name-value modal style.
- If the artifact cannot be loaded, show an actionable error in the modal; the log row stays visible and usable. Nonmatching keys are plain text.

## Error handling

- Invalid filter or pagination values return a clear client error; database failures return the project-standard server error.
- The view shows API failures inline and clears stale rows.
- Dialog fetch failures are contained to the dialog and do not reload or discard the table.

## Tests and verification

- Add store and handler tests for pagination, ordering, filters, JSON row fields, and finding-count edge cases.
- Add focused frontend checks or component-level tests for artifact-ID recognition, findings display, and dialog data preparation where the project tooling supports them.
- Run relevant Go tests and the web type/lint/build verification used by ChenWeb.

## Documentation impact

- This spec records the new admin capability and its API/UI contract.
- Existing user-facing documents are unaffected; no operational schema documentation changes because the table already exists.
