# Reset Search Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan.

**Goal:** Add a `Reset Search` button immediately after `Search` on the Knowledge Base Import Inputs view.

**Architecture:** Reuse the existing `loadRecords` path. The reset handler clears every filter populated by the search dialog, resets pagination to page 1, and reloads the unfiltered list.

**Tech Stack:** Svelte 5, TypeScript, SvelteKit, existing `kb-import-view.svelte` state and `listKbInputs` service.

---

### Task 1: Add reset behavior and control

**Files:**
- Modify: `web/src/lib/components/home3/kb-import-view.svelte`

- [x] Add a `resetSearch` handler that clears record ID, title, document number, file name, type, parser, pipeline/process filters, date ranges, parse state, and resets the page.
- [x] Add a `Reset Search` button directly after the existing `Search` button, matching the existing secondary button styling and disabling it while records are loading.
- [x] Run the frontend Svelte check; it passes with zero errors. Project-local ESLint and Prettier still report pre-existing issues elsewhere in this already-dirty component, so the file was not reformatted wholesale.

### Task 2: Verify the user flow

**Files:**
- None

- [x] Confirm the reset handler routes through `loadRecords`, so deleting the last record in a filtered result can be followed by resetting to all records without changing backend behavior.
- [x] Review the diff to ensure no unrelated files or behavior changed.
