# Resolve Orphaned Ontology Term Labels

## Problem

Rows in `kb.ontology_term_labels` can outlive their referenced ontology term. These orphaned labels are not resolvable and can cause ontology term identity writes to fail with duplicate preferred-label errors, such as the `extract_metrics` failure reported on 2026-08-19.

## Scope

Add a System Admin → Database Maintenance page named `Resolve Orphaned Labels` that lists orphaned label rows and removes the rows selected by the current search result.

An orphaned label is a row in `kb.ontology_term_labels` whose `term_id` has no matching `term_id` in `kb.ontology_terms`.

## Options considered

1. Add dedicated database-maintenance endpoints and a dedicated Svelte view. Recommended: explicit, auditable, and consistent with the existing `dbmainthandler` maintenance endpoints.
2. Reuse the ontology term stores. Rejected because it adds indirection and makes destructive maintenance behavior less obvious.
3. Build a generic database-maintenance framework. Rejected as unnecessary scope for this targeted repair.

## Design

### Backend

- Add a `GET` endpoint under `/api/v1/admin/db/` that lists orphaned labels.
- Return `id`, `term_id`, `label`, `lang`, `label_role`, `status`, and audit timestamps.
- Support a free-text query across `term_id`, `label`, and `lang`, plus optional exact `lang` and `label_role` filters.
- Add a `POST` endpoint that accepts the currently listed orphan label IDs and deletes only rows that are still orphaned. The deletion is transactional.
- If the submitted ID list is empty, perform no deletion and return a zero count.
- Record the operation and deleted count in `kb.db_maintenance_logs`, including the applied filters.

### Frontend

- Add `Resolve Orphaned Labels` to the existing Database Maintenance navigation group.
- Add a view under `web/src/lib/components/home3` following the existing maintenance-page tokens and layout.
- Explain the orphan condition in plain language at the top of the page.
- Provide one free-text search field across `term_id`, `label`, and `lang`, with optional language and label-role filters.
- Render results in a scrollable table with the identifying label fields and status.
- Disable Resolve when there are no listed records; require confirmation before deletion.
- After a successful resolve, reload the list and show how many rows were removed.
- Show loading, empty, request-error, and resolve-error states.

### Data flow

1. The view loads orphaned labels on mount.
2. Search reloads the list with the current filters.
3. Resolve submits the IDs currently shown by the filtered result.
4. The server rechecks orphan status in the delete query, deletes matching rows in a transaction, logs the result, and returns the count.
5. The view reloads and displays the updated empty or remaining result set.

### Error handling and safety

- The API returns service-unavailable when the project database is unavailable and a structured error for query or transaction failures.
- SQL parameters are used for all user-supplied values.
- The delete statement includes the orphan predicate so a label that was repaired between listing and resolving is preserved.
- The UI requires explicit confirmation and never sends a delete request for an empty list.

### Verification

- Add handler SQL-mock tests for orphan-only listing, free-text/language/role filtering, deletion scoping, empty submission, and maintenance logging.
- Add frontend client tests for query-string construction and resolve payload handling.
- Run the focused Go and frontend tests, then the relevant ChenWeb build/type-check commands.

## Intentionally not included

- Repairing or recreating ontology terms.
- Deleting non-orphaned labels.
- Pagination; the first version is intended for a maintenance queue and will use the existing page’s scrollable list pattern.
- A generic framework for future maintenance checks.
