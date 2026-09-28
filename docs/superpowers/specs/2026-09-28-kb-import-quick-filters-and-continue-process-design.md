# Knowledge Base Import Quick Filters and Continue Process

## Goal

Make the Import Inputs toolbar easier to use for finding records that need work and continuing only the relevant processing operations.

## User interface

Replace the Show Failed button with a Quick Filters dropdown containing:

- **Show Failed**: filter to inputs whose status contains failed entries.
- **Show no doc processors**: filter to inputs with no status operation matching any registered doc processor name. Parse/conversion and other control operations do not count as processor runs, even when they have rows in `kb.input_proc_status`; a generic `doc_processing` entry without a processor name does not count. Missing or empty status matches.

Rename the Process Parsed dropdown to **Continue Process**, with these actions:

- Process Parsed - All
- Process Parsed - Selected
- Process Failed - All
- Process Failed - Selected

Both Selected actions are disabled when no records are selected. Selected actions are disabled when the selection is empty and operate on selected record IDs. All actions operate across all records matching the current search and filters, not only the current page.

## Processing behavior

Parsed actions keep their existing behavior: queue conversion when conversion has not completed, otherwise start doc processing.

Failed actions target records with failed doc processor operations and retry only those failed processors. Previously successful processors must not be rerun. The All variant intersects these records with the current search and filters; the Selected variant intersects them with selected IDs and current search and filters.

## Implementation boundaries

Keep the changes in the Knowledge Base Import Inputs frontend and existing backend/API processing surfaces where possible. Reuse the existing status representation and retry/event mechanisms; add backend support only where current APIs cannot identify failed operations or enqueue only those operations.

## Verification

Check failed-status filtering, the no-doc-processors condition, each All/Selected action, empty-selection disabled states, and failed-operation-only retries. Run the frontend lint and Svelte type checks required by `web/CLAUDE.md`.
