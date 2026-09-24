# Knowledge Upload Files: Multi-Record Delete

## Goal

In `home3/knowledge` → `File Management` → `Upload Files`, allow users to select multiple records visible on the current page and delete them together. The existing single-record delete behavior remains unchanged.

## Design

- Add a checkbox column to the records table.
- Add a header checkbox that selects or clears all records currently visible on the page.
- Add a page-level `Delete` action when at least one record is selected.
- Reuse the existing delete confirmation dialog and its existing option to delete the physical file and generated artifacts.
- When confirmed, call the existing `DELETE /api/v1/kb/inputs/:id` endpoint once per selected record, passing the same `delete_file=true` query parameter for every request when the option is enabled.
- Selection is page-local. Pagination, filtering, sorting, and reloads clear the selection.

## Error handling

Batch deletion stops at the first failed request and displays the existing delete error. Records successfully deleted before the failure remain deleted. The list is reloaded after the operation so the UI reflects the server state.

## Scope

No server endpoint or database change is required. No selection across pages, background job, undo action, or new deletion semantics are introduced.

## Verification

- Add or update frontend tests for selecting visible records, toggling the page checkbox, and applying the same file-deletion option to every selected delete request when the project test setup supports these component/service assertions.
- Run the relevant frontend checks and existing KB handler tests.
- Run the ChenWeb build/type-check path used by the repository before completion.
