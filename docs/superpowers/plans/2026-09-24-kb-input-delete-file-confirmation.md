# KB Input Delete File Confirmation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users delete a `kb.inputs` record without deleting its physical files by default, with an explicit opt-in in the Upload Files confirmation dialog.

**Architecture:** Keep record deletion on the existing endpoint. The optional `delete_file=true` parameter archives the PDF and removes its backup and generated artifacts; omitted or false preserves these files. The Svelte dialog owns the unchecked opt-in state and passes it through the service.

**Tech Stack:** Svelte 5, TypeScript, Go, Echo, sqlmock.

---

### Task 1: Lock down backend deletion semantics

**Files:**
- Modify: `server/api/kbhandler/metrics_handler.go`
- Test: `server/api/kbhandler/metrics_handler_test.go`

- [x] Add a failing test proving the default delete request does not invoke file cleanup while still deleting the database record.
- [x] Add a failing test proving `delete_file=true` invokes the existing file and artifact cleanup after commit.
- [x] Run the focused Go tests and confirm the new tests fail for the missing request behavior.
- [x] Parse an optional `delete_file` query parameter, defaulting to false, and guard `deleteInputFiles` and `deleteInputArtifactDirs` with it.
- [x] Run the focused Go tests and confirm they pass.

### Task 2: Add the UI opt-in

**Files:**
- Modify: `web/src/lib/services/kbService.ts`
- Modify: `web/src/lib/components/home3/kb-import-view.svelte`

- [x] Add an optional service argument for the delete-file choice and serialize it as `delete_file=true` only when selected.
- [x] Add an unchecked checkbox to the existing delete confirmation dialog with explicit wording that the physical file will also be deleted.
- [x] Pass the checkbox state when confirming deletion and reset it whenever the dialog opens/closes.
- [x] Run Svelte type checking and formatting/lint checks for the touched files.

### Task 3: Final verification

**Files:**
- No additional files.

- [x] Run the focused Go package tests.
- [x] Run the web checks.
- [x] Review the final diff to ensure unrelated pre-existing changes remain untouched.
- [ ] Commit only the plan and implementation files for this task through `jj` if the repository state permits an isolated commit.

## Archive and audit corrections (2026-10-19)

- The PDF moves to `ARCHIVE_DIR/<group>/<record id>/<name>`. Collisions use numbered suffixes and exclusive destination creation; permission errors return immediately. Database or audit-insert failure restores the source PDF. An already-missing PDF remains nonfatal and appears in `missing_files`.
- Cleanup accepts only the record's expected directories under configured artifact roots. It records each successful file removal, including partial directory cleanup and removed ArtifactWeb index and metadata files.
- A `delete_input` audit row is inserted in the same transaction as the input deletion, before file cleanup. PostgreSQL `pg_stat_xact_user_tables` supplies actual table deletion counts, including cascades; `track_counts` must be enabled. Failure to collect counts or insert the audit rolls back deletion.
- `extra_info.cleanup_status` starts as `pending`, with the archive path, deleted table counts, planned files/directories and ArtifactWeb roots. Finalization merges actual `deleted_files`, `missing_files`, ArtifactWeb results and a `completed` or `failed` status; failures also populate `errors`. A failed final update returns an error and leaves the durable pending entry. Database and filesystem changes cannot commit atomically; a pending entry requires reconciliation after an interrupted operation. Cleanup retries preserve deletion audit rows.
- The English and Chinese confirmation labels describe archival and backup/artifact deletion. The existing message key remains for compatibility.
- Regression coverage includes permission errors, archive collisions, partial removal, ArtifactWeb filenames, and audit insertion/finalization failure. `TestDeleteInputPostgres` exercises real cascade counts and finalized audit contents when `KB_INPUT_DELETE_TEST_DSN` points to an empty disposable PostgreSQL database without a `kb` schema.
- No schema or public API changes were needed. Historical implementation checklists above are retained as history; no separate ADR was added for this correction.
