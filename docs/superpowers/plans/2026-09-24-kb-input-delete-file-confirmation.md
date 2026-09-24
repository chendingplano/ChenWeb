# KB Input Delete File Confirmation Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users delete a `kb.inputs` record without deleting its physical files by default, with an explicit opt-in in the Upload Files confirmation dialog.

**Architecture:** Keep record deletion on the existing endpoint. Add an optional boolean request parameter to control post-transaction file and artifact-directory cleanup; omitted or false preserves files. The Svelte dialog owns the unchecked opt-in state and passes it through the service.

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
