# Knowledge Upload Files Multi-Delete Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add current-page multi-selection and batch deletion to the Upload Files list while applying the existing file-deletion option to every selected record.

**Architecture:** Keep deletion server-side semantics unchanged. Add page-local selection state and a shared batch confirmation flow in `kb-import-view.svelte`; issue the existing `deleteKbInput(id, deleteFile)` request sequentially for selected IDs.

**Tech Stack:** Svelte 5, TypeScript, existing `kbService.ts` fetch service, Vite/Svelte checks, Go test suite for the existing delete handler.

---

### Task 1: Add selection state and batch deletion flow

**Files:**
- Modify: `web/src/lib/components/home3/kb-import-view.svelte`

- [x] Add a page-local selected-ID set, header checkbox helpers, and clear selection when records reload or list navigation changes.
- [x] Extend the existing confirmation state to represent either one record or selected records without changing single-delete behavior.
- [x] Implement sequential deletion using the existing service function and shared `deleteFileToo` flag.
- [x] Add the checkbox column, current-page select-all control, selected-count Delete action, and confirmation copy.

### Task 2: Verify service and UI behavior

**Files:**
- Inspect: `web/src/lib/services/kbService.ts`
- Inspect: existing frontend package scripts and tests

- [x] Confirm the existing `deleteKbInput(id, deleteFile)` service produces `delete_file=true` for each batch request and no query parameter otherwise.
- [x] Add focused tests for the page-selection rules using Bun, since the repository has no configured component-test suite.
- [x] Run the relevant frontend check/build and existing `server/api/kbhandler` tests.
- [x] Review the diff to ensure pre-existing user changes remain intact and no unrelated files are modified.
