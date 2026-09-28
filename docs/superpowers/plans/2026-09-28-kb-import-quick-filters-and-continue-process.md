# KB Import Quick Filters and Continue Process Implementation Plan

> **For agentic workers:** Implement each checklist step in order and keep the changes scoped to the Import Inputs UI and KB input query/retry paths.

**Goal:** Add the requested Quick Filters and Continue Process menus, including filtered retries of only failed doc processor operations.

**Architecture:** Extend the existing KB inputs filtering/query path for “no doc processors” and for collecting all IDs that match the active Import Inputs filters. Keep selected actions ID scoped. Reuse the existing doc processing event's failed-processor-only behavior for retries, so successful processors are not rerun.

**Tech Stack:** Svelte 5, TypeScript, Go, Echo, PostgreSQL JSONB and KB input status rollups.

---

## Files and responsibilities

- `web/src/lib/components/home3/kb-import-view.svelte`: toolbar dropdowns, active filter state, bulk action selection, and event publishing.
- `web/src/lib/services/kbService.ts`: typed query parameters and API request serialization for no-processor and filtered ID lookups.
- `server/api/kbhandler/handler.go`: input-list filter parsing and SQL; add a filtered all-matching ID response for bulk operations if existing API pagination is insufficient.
- `server/api/kbhandler/handler_test.go`: SQL builder coverage for no-processor filtering and failed-input constraints.
- `docs/superpowers/specs/2026-09-28-kb-import-quick-filters-and-continue-process-design.md`: approved behavior.

## Chunk 1: Filter model and query support

### Task 1: Add no-doc-processors filtering to KB inputs

**Files:**
- Modify: `server/api/kbhandler/handler.go`
- Modify: `web/src/lib/services/kbService.ts`
- Modify: `web/src/lib/components/home3/kb-import-view.svelte`
- Modify: `server/api/kbhandler/handler_test.go`

- [ ] Replace the old Show Failed boolean with one Quick Filters selection (`none`, `failed`, or `no_doc_processors`). The failed filter matches status entries whose normalized status is exactly `failed` (including parse failures). No-doc-processors checks for absence of a `kb.input_proc_status` row whose processor is in the registered doc processor aliases from `getAllProcessorAliases`; parse/conversion/control rows do not count, and a generic aggregate `doc_processing` entry does not count.
- [ ] Pass the single Quick Filters selection into `listKbInputs`, replacing the existing Show Failed boolean.
- [ ] Preserve current search and filter state when either quick filter is selected; reset pagination and reload.
- [ ] Confirm query behavior for missing/empty status, aggregate-only status, and parsed/converted-only history; all match no-doc-processors, while any registered processor status row excludes the record.

### Task 2: Collect all filtered IDs for bulk actions

**Files:**
- Modify: `server/api/kbhandler/handler.go`
- Modify: `server/api/routes.go` only if a new route is needed
- Modify: `web/src/lib/services/kbService.ts`
- Modify: `web/src/lib/components/home3/kb-import-view.svelte`

- [ ] Reuse `ListInputs` filter parsing, `buildWhereClause`, and authorization rules to return all IDs matching current search/filters without page truncation; factor shared query-filter parsing if needed rather than copy its SQL. Keep the ID endpoint scoped to all matching rows, independent of pagination.
- [ ] For Process Failed - All, intersect those IDs with `i.has_failed_proc` / named failed processor rows, excluding parse, conversion, aggregate control, and stopped/unknown statuses.
- [ ] For Process Parsed - All, preserve the existing parsed-success, `pdf_parsing`, and post-parse conversion/doc-processing eligibility rules and `converted_after_parse` result while intersecting with current list filters. Continue requesting the existing process-parsed endpoint with scoped IDs so it remains the eligibility authority.
- [ ] For Selected actions, intersect selected IDs with current search/filter matches so an active search/filter continues to scope the action; ensure the selected ID path still runs through each action’s eligibility checks.

## Chunk 2: Retry behavior and menus

### Task 3: Add failed processor retry publishing

**Files:**
- Modify: `web/src/lib/components/home3/kb-import-view.svelte`
- Reuse existing: `server/api/doc-processing/start_doc_processing_event.go`, `server/api/doc-processing/control.go`

- [ ] Add All and Selected failed retry handlers that publish scoped record ID batches with `failed-proc-only: true`; never use auto mode or the global `all: failed-procs` selector for filtered bulk retries.
- [ ] Ensure the event retries only failed named doc processor operations; rely on existing `failedDocProcessorNames` selection and do not rerun successes. Parsing/conversion failures remain visible through Show Failed but are not targeted by Process Failed.
- [ ] Report queued and failed counts using the existing Process Parsed feedback UI patterns.
- [ ] Reload the list after queueing so status is refreshed.

### Task 4: Replace toolbar controls

**Files:**
- Modify: `web/src/lib/components/home3/kb-import-view.svelte`

- [ ] Replace Show Failed button with a Quick Filters dropdown containing Show Failed and Show no doc processors.
- [ ] Rename Process Parsed menu to Continue Process and expose all four requested actions.
- [ ] Disable both Selected menu actions when there are no selected records; retain disabled/loading behavior during an active bulk action.
- [ ] Close menus after choosing an action and reflect the active quick filter in the label or selected styling.

## Chunk 3: Verification and documentation

### Task 5: Verify implementation

**Files:**
- Verify: `web/src/lib/components/home3/kb-import-view.svelte`
- Verify: `web/src/lib/services/kbService.ts`
- Verify: `server/api/kbhandler/handler.go`

- [ ] Run the required frontend ESLint check on the modified Svelte/TypeScript files.
- [ ] Run the required Svelte type check and confirm there are no errors attributable to the modified files.
- [ ] Review the final diff against the approved spec, including current-filter intersection, no-doc-processors semantics, failed-only retries, and empty-selection disabled states.
- [ ] Record the changed knowledge and documentation impact in the completion summary.
