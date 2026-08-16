# Semantic Decision Candidates Admin Page Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a lifecycle-safe CRUD-style admin page for `kb.semantic_decision_candidates` at System Admin → Doc Process Pipeline → Semantic Decision Candidates, with no Delete operation.

**Architecture:** Extend the existing assertions decision-candidate domain store with a paginated filterable list method, then add dedicated Echo handlers for create/read/list and lifecycle mutations. Add a typed frontend client and Svelte admin view, wired through the existing home3 navigation and content panel.

**Tech Stack:** Go, Echo, database/sql, go-sqlmock, Svelte 5 runes, TypeScript, Tailwind utility classes, Lucide icons.

---

## Chunk 1: Backend store and API

### Task 1: Add paginated candidate listing to the domain store

**Files:**
- Modify: `server/api/ontology/assertions/decision_candidates_store.go`
- Test: `server/api/ontology/assertions/decision_candidates_store_test.go` (create if absent)

- [ ] Add a list filter/input type with optional status, candidate kind, method, logical identity, source artifact type/id, input record ID, page, page size, and allow-listed order direction.
- [ ] Add a row-count query and data query using parameterized filters, deterministic `id DESC` ordering, and bounded page/page-size values.
- [ ] Reuse `scanDecisionCandidate` and return rows, total, and database errors without changing existing lifecycle methods.
- [ ] Add sqlmock coverage for all filters, pagination offset, count, and count/query errors.
- [ ] Run `go test ./server/api/ontology/assertions -run DecisionCandidate` and confirm the new tests pass.

### Task 2: Add lifecycle-safe HTTP handlers

**Files:**
- Create: `server/api/kbhandler/semantic_decision_candidates_handler.go`
- Test: `server/api/kbhandler/semantic_decision_candidates_handler_test.go`

- [ ] Implement list and detail response DTOs using the existing `{status, error_msg}` convention.
- [ ] Implement GET list parsing/validation and call the new store list method.
- [ ] Implement POST create by decoding `DecisionCandidate`, requiring JSON payload and identity/kind/method, and calling `Propose`.
- [ ] Implement GET detail by ID.
- [ ] Implement POST transition, resolution, defer, retry, and assertion-link actions by delegating to the existing store methods.
- [ ] Return 400 for malformed IDs/bodies and domain validation/illegal transitions, 404 for missing rows where appropriate, and log failures with `EchoFactory.NewFromEcho`.
- [ ] Add handler sqlmock tests for list response, create validation, detail, transition, resolution, defer/retry, and assertion-link request shapes.
- [ ] Run `go test ./server/api/kbhandler -run SemanticDecisionCandidate` and confirm the tests pass.

### Task 3: Register backend routes

**Files:**
- Modify: `server/api/routes.go`

- [ ] Register GET/POST collection routes and GET/action routes under `/kb/semantic-decision-candidates` beside the other Doc Process Pipeline APIs.
- [ ] Run `go test ./server/api/...` to catch route-package compilation and handler regressions.

## Chunk 2: Frontend API and page

### Task 4: Add typed frontend client

**Files:**
- Create: `web/src/lib/components/home3/semantic-decision-candidates-client.ts`
- Test: `web/src/lib/components/home3/semantic-decision-candidates-client.test.ts` (if existing client-test conventions support it)

- [ ] Define candidate, list response, create input, and action input types matching the backend JSON.
- [ ] Implement authenticated request helpers for list, detail, create, transition, resolution, defer, retry, and assertion-link.
- [ ] Normalize non-OK responses into readable errors using the existing frontend service conventions.
- [ ] Add focused URL/body/error parsing tests where practical.

### Task 5: Build the Svelte admin view

**Files:**
- Create: `web/src/lib/components/home3/semantic-decision-candidates-view.svelte`

- [ ] Implement Svelte 5 state for loading, errors, rows, pagination, filters, create/detail dialogs, and selected record.
- [ ] Render filters for status, kind, method, logical identity, source artifact, and input record; support apply/clear/refresh/pagination.
- [ ] Render a horizontally scrollable table with lifecycle metadata and compact JSON/source information.
- [ ] Implement create dialog with JSON validation for `proposed_payload` and optional `source_line_spans`, calling `Propose` through the client.
- [ ] Implement detail dialog with read-only generated fields and lifecycle-safe action forms for legal transitions, resolution, defer/retry, and assertion linking.
- [ ] Ensure there is no Delete button, delete handler, or delete request.
- [ ] Match existing admin page dark/light tokens, loading/error states, keyboard-close behavior, and internal scrolling.

## Chunk 3: Navigation and verification

### Task 6: Wire the page into Doc Process Pipeline

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`
- Modify: `web/src/lib/components/home3/content-panel.svelte`

- [ ] Add `sysadmin-doc-process-semantic-decision-candidates` under the existing `sysadmin-doc-process-pipeline` group with label `Semantic Decision Candidates`.
- [ ] Import the new view and render it for the new child ID.
- [ ] Include the new child ID in the app-shell/footer behavior if needed for full-height scrolling.

### Task 7: Verify the complete change

**Files:**
- Inspect: `git diff`, `jj status`, and changed-file list.

- [ ] Run `gofmt` on changed Go files.
- [ ] Run `go test ./server/api/ontology/assertions ./server/api/kbhandler`.
- [ ] Run the project frontend check from `web` (`bun run check`, or the repository’s documented equivalent if unavailable).
- [ ] Confirm navigation text and route ID with `rg`.
- [ ] Confirm no changed file adds a Delete action or DELETE endpoint for semantic decision candidates.
- [ ] Review changed files for unrelated modifications and leave pre-existing dirty work untouched.
- [ ] Commit implementation changes through `jj`, then run `jj log` and verify the expected linear commit.
