# Chad Session Viewer Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an authenticated, read-only ChenWeb admin page named Chad Sessions that lists every local Chad session and renders its request/response log.

**Architecture:** Add a focused Go `chadsessionshandler` that safely reads the server user's `~/.chad/sessions` store through two authenticated JSON endpoints. Add a typed browser client and a full-height Svelte two-pane view, then connect the view to the existing `/development` nav rail and content panel under System Admin → LLM.

**Tech Stack:** Go, Echo, Go `encoding/json`/`os`, Svelte 5, TypeScript, existing ChenWeb dashboard tokens and authenticated `/api/v1` route group, Vitest frontend tests.

---

## Chunk 1: Backend session API

### Task 1: Map the existing handler and route test conventions

**Files:**
- Read: `server/api/routes.go`
- Read: `server/api/chatterhandler/handler.go`
- Read: `server/api/routes_proxytrace_test.go`
- Read: one existing focused handler test in `server/api/llmadminhandler/handler_test.go`

- [ ] **Step 1: Confirm the authenticated route registration shape and response conventions.**

  Record the exact `apiGroup` registration point, handler function signatures, JSON error shape, and test helper pattern before editing.

- [ ] **Step 2: Run the focused existing Go tests as a baseline.**

  Run: `go test ./server/api/...`

  Expected: existing package tests pass (or any pre-existing failures are recorded without changing unrelated files).

### Task 2: Add testable session-store parsing and HTTP behavior

**Files:**
- Create: `server/api/chadsessionshandler/handler.go`
- Create: `server/api/chadsessionshandler/handler_test.go`

- [ ] **Step 1: Write failing tests for valid summaries and newest-first ordering.**

  Use a temporary home directory with `.chad/sessions/<id>/index.json` and session JSON fixtures matching the observed `session_id`, `cwd`, `updated`, `meta`, and `messages` shape. Assert the list response contains summaries sorted by numeric update time descending and message counts.

- [ ] **Step 2: Write failing tests for detail retrieval and missing sessions.**

  Assert a valid id returns metadata and messages, while an unknown id returns `404` with a JSON error.

- [ ] **Step 3: Write failing tests for path validation and malformed entries.**

  Assert ids containing `/`, `\\`, `.`, or `..` are rejected, and malformed/unreadable individual session files do not prevent valid sessions from appearing in the list.

- [ ] **Step 4: Write a failing test for the response-size bound.**

  Assert a detail request for a session file larger than the named maximum returns a clear client error rather than reading/encoding an unbounded payload.

- [ ] **Step 5: Run the new tests to verify they fail for the missing handler.**

  Run: `go test ./server/api/chadsessionshandler -v`

  Expected: FAIL because the handler/store implementation does not yet exist.

- [ ] **Step 6: Implement the minimal store reader and Echo handlers.**

  Add typed response structs, a named maximum file size, safe home/session-directory resolution, direct-child session-id validation, JSON decoding, summary extraction, newest-first sorting, and isolated list-entry failures. Keep the store logic in the same focused package unless the tests demonstrate a separate file is needed.

- [ ] **Step 7: Register the two routes behind the existing authenticated API group.**

  Modify: `server/api/routes.go`

  Import the handler package and register `GET /chad/sessions` and `GET /chad/sessions/:id` adjacent to the existing chatter routes, preserving `apiGroup.Use(authmiddleware.AuthMiddleware)` coverage.

- [ ] **Step 8: Run the handler tests and route package tests.**

  Run: `go test ./server/api/chadsessionshandler ./server/api`

  Expected: PASS.

- [ ] **Step 9: Commit the backend slice with jj.**

  Run: `jj commit server/api/chadsessionshandler server/api/routes.go -m 'feat: expose Chad sessions for admin viewing'`

### Task 3: Add typed frontend API client tests and implementation

**Files:**
- Create: `web/src/lib/components/home3/chad-sessions-client.ts`
- Create: `web/src/lib/components/home3/chad-sessions-client.test.ts`

- [ ] **Step 1: Write failing client tests for list/detail URLs and typed results.**

  Mock `fetch`, call the exported list and detail functions, and assert the exact `/api/v1/chad/sessions` paths, credentials behavior, and returned data.

- [ ] **Step 2: Write a failing client test for HTTP and non-JSON errors.**

  Assert an HTTP error becomes a useful `Error` even when the response body is not JSON.

- [ ] **Step 3: Run the focused frontend tests to verify they fail.**

  Run from `web`: `npm test -- chad-sessions-client.test.ts`

  Expected: FAIL because the client module does not yet exist.

- [ ] **Step 4: Implement the typed client.**

  Export summary/detail/message types and small `listChadSessions()` / `getChadSession(id)` functions following the existing client fetch conventions.

- [ ] **Step 5: Run the focused frontend tests.**

  Run from `web`: `npm test -- chad-sessions-client.test.ts`

  Expected: PASS.

- [ ] **Step 6: Commit the frontend API slice with jj.**

  Run: `jj commit web/src/lib/components/home3/chad-sessions-client.ts web/src/lib/components/home3/chad-sessions-client.test.ts -m 'feat: add Chad sessions API client'`

## Chunk 2: Admin UI and navigation

### Task 4: Build the session viewer component

**Files:**
- Create: `web/src/lib/components/home3/chad-sessions-view.svelte`

- [ ] **Step 1: Implement loading, empty, error, and selection state.**

  Load summaries on mount, select the first available session, preserve selection across refresh where possible, and show actionable list/detail error states.

- [ ] **Step 2: Implement the two-pane session list/detail layout.**

  Show title/description/count/refresh, newest-first session rows with id/time/cwd/message count, and a selected-session detail header.

- [ ] **Step 3: Implement safe message rendering.**

  Render all content as escaped text. Display string content directly and arrays/objects with formatted JSON. Give system, user, and assistant entries distinct labels/treatment without assuming only those roles exist.

- [ ] **Step 4: Add responsive and dark-mode styling consistent with System Admin views.**

  Use the existing `darkMode` prop and local color tokens, keep the main shell full-height, and stack the panes at narrow widths.

- [ ] **Step 5: Run the frontend typecheck/build.**

  Run from `web`: `npm run check` (or the repository’s existing frontend check command if the package scripts use a different name).

  Expected: PASS with no new diagnostics.

### Task 5: Wire the viewer into the dashboard

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`
- Modify: `web/src/lib/components/home3/content-panel.svelte`

- [ ] **Step 1: Add the `sysadmin-llm-chat-sessions` leaf.**

  Place `Chad Sessions` beneath the existing `sysadmin-llm` group after the model entries, so the rendered hierarchy is Development → System Admin → LLM → Chad Sessions.

- [ ] **Step 2: Import and render the new viewer.**

  Add the component import and a branch for `activeMenu?.childId === 'sysadmin-llm-chat-sessions'`.

- [ ] **Step 3: Include the viewer in the app-shell/footer rules.**

  Add the new child id to the no-footer/full-height set next to the existing LLM usage log and other operational screens.

- [ ] **Step 4: Run frontend tests/checks.**

  Run from `web`: `npm test` and `npm run check`.

  Expected: PASS.

- [ ] **Step 5: Commit the UI/navigation slice with jj.**

  Run: `jj commit web/src/lib/components/home3/chad-sessions-view.svelte web/src/lib/components/home3/nav-rail.svelte web/src/lib/components/home3/content-panel.svelte -m 'feat: add Chad sessions admin page'`

## Chunk 3: Integration verification and handoff

### Task 6: Verify the complete feature

**Files:**
- Read/verify: `server/api/chadsessionshandler/handler.go`
- Read/verify: `web/src/lib/components/home3/chad-sessions-view.svelte`
- Read/verify: `web/src/lib/components/home3/nav-rail.svelte`
- Read/verify: `web/src/lib/components/home3/content-panel.svelte`

- [ ] **Step 1: Run the focused backend tests.**

  Run: `go test ./server/api/chadsessionshandler ./server/api`

  Expected: PASS.

- [ ] **Step 2: Run the complete relevant frontend checks.**

  Run from `web`: `npm test` and `npm run check`.

  Expected: PASS.

- [ ] **Step 3: Run the ChenWeb build or project verification command.**

  Run: `mise build-server` from `ChenWeb` if available; otherwise use the documented project build command.

  Expected: successful server/frontend build.

- [ ] **Step 4: Manually verify with a running authenticated app.**

  Open `/development`, expand `System Admin → LLM`, confirm `Chad Sessions` appears, select a real session, confirm request/response content renders, refresh, and verify a session with structured content does not execute markup.

- [ ] **Step 5: Answer the workspace documentation checklist.**

  Knowledge changed: the ChenWeb admin UI now exposes local Chad session logs. Affected docs/specs/tests: this design/plan, handler tests, client tests, and existing navigation/content code. No end-user manual update is needed unless the manual documents the System Admin menu; if it does, update that page. Leave unrelated dirty work untouched.

- [ ] **Step 6: Inspect jj history and final status.**

  Run: `jj log -n 6 --no-pager` and `jj status`

  Confirm the feature commits are linear and unrelated pre-existing modifications remain uncommitted.
