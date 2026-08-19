# Resolve Orphaned Ontology Term Labels Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a System Admin database-maintenance page that finds orphaned `kb.ontology_term_labels` rows, scopes them with search filters, and deletes only the currently listed orphan rows after confirmation.

**Architecture:** Add two explicit endpoints to `dbmainthandler`, using a `NOT EXISTS` orphan predicate against `kb.ontology_terms`. Add a small TypeScript API client and a Svelte maintenance view, then register the view and navigation item in the existing `/development` shell. Resolve rechecks the orphan predicate inside a transaction and writes the existing maintenance log.

**Tech Stack:** Go, Echo, `database/sql`, sqlmock, Svelte 5, TypeScript, Tailwind utility classes plus the existing inline design tokens, Vitest.

---

## Chunk 1: Backend API and tests

### Task 1: Define the orphan-label API contract and failing tests

**Files:**
- Create: `server/api/dbmainthandler/orphaned_labels_test.go`
- Modify: `server/api/dbmainthandler/handler.go`

- [ ] **Step 1: Add SQL-mock test for listing only orphaned labels**

  Cover a row returned when `NOT EXISTS (SELECT 1 FROM kb.ontology_terms ...)` is true, and assert the response JSON includes the label identity and audit fields.

- [ ] **Step 2: Add SQL-mock test for free-text, language, and role filters**

  Send `q`, `lang`, and `label_role` query parameters. Assert the generated query includes parameterized predicates and the expected arguments.

- [ ] **Step 3: Add SQL-mock tests for resolve behavior**

  Cover: deleting submitted orphan IDs, returning the affected count, refusing an empty ID list without touching the database, and leaving a now-non-orphaned row untouched because the delete query repeats the orphan predicate. Expect a maintenance-log insert after a successful delete.

- [ ] **Step 4: Run the new tests to verify they fail for missing handlers**

  Run: `go test ./server/api/dbmainthandler -run 'Test(List|Resolve)OrphanedLabels' -count=1`

  Expected: FAIL because the endpoint functions and response types do not yet exist.

### Task 2: Implement the backend endpoints

**Files:**
- Modify: `server/api/dbmainthandler/handler.go`
- Modify: `server/api/routes.go:741-745`

- [ ] **Step 1: Add response and request types**

  Define an `OrphanedLabelRow` matching the required JSON fields and a request containing `ids []int64` plus optional filter values for the audit record.

- [ ] **Step 2: Implement `ListOrphanedLabels`**

  Build a parameterized `WHERE` clause for the orphan `NOT EXISTS` predicate, optional case-insensitive free-text matching over `term_id`, `label`, and `lang`, and exact `lang`/`label_role` filters. Return `results` and `total`, with a deterministic order by `term_id`, `lang`, `label_role`, and `id`.

- [ ] **Step 3: Implement `ResolveOrphanedLabels`**

  Decode and validate the JSON body, return zero for an empty ID list, begin a transaction, delete only submitted IDs that still satisfy the orphan predicate, insert a maintenance log in the same transaction, commit, and return `deleted_count`.

- [ ] **Step 4: Register the admin routes**

  Add `GET /admin/db/ontology-term-labels/orphans` and `POST /admin/db/ontology-term-labels/orphans/resolve` next to the other database-maintenance routes.

- [ ] **Step 5: Run the focused backend tests**

  Run: `go test ./server/api/dbmainthandler -run 'Test(List|Resolve)OrphanedLabels' -count=1`

  Expected: PASS.

### Task 3: Run backend package verification

- [ ] **Step 1: Run all database-maintenance handler tests**

  Run: `go test ./server/api/dbmainthandler -count=1`

- [ ] **Step 2: Run a ChenWeb server build**

  Run: `go build ./server/...`

  Expected: exit 0.

## Chunk 2: Frontend client and view

### Task 4: Add the frontend client with failing tests

**Files:**
- Create: `web/src/lib/components/home3/resolve-orphaned-labels-client.ts`
- Create: `web/src/lib/components/home3/resolve-orphaned-labels-client.test.ts`

- [ ] **Step 1: Write client helper tests**

  Test query construction for blank filters and for `q`, `lang`, and `label_role`; test that resolve sends a JSON body containing the visible row IDs and returns the server count.

- [ ] **Step 2: Run the tests to verify they fail**

  Run from `web`: `bunx vitest run src/lib/components/home3/resolve-orphaned-labels-client.test.ts`

  Expected: FAIL because the client module is absent.

- [ ] **Step 3: Implement the typed API client**

  Add row/filter/request types, a shared same-origin request helper matching existing clients, `buildOrphanedLabelsQuery`, `listOrphanedLabels`, and `resolveOrphanedLabels`.

- [ ] **Step 4: Run the client tests**

  Run from `web`: `bunx vitest run src/lib/components/home3/resolve-orphaned-labels-client.test.ts`

  Expected: PASS.

### Task 5: Build the maintenance view

**Files:**
- Create: `web/src/lib/components/home3/resolve-orphaned-labels-view.svelte`

- [ ] **Step 1: Add state and load behavior**

  Track rows, filters, loading, errors, and resolving state. Load on mount and reload after a successful resolve.

- [ ] **Step 2: Add the explanation and search area**

  Explain the orphan condition in plain language. Add free-text, language, and label-role controls plus Search.

- [ ] **Step 3: Add the results table and empty state**

  Show count, term ID, label, language, role, status, and timestamps in a scrollable panel. Render a no-orphans message when the response is empty.

- [ ] **Step 4: Add the Resolve action**

  Disable Resolve for an empty result set or while resolving. Confirm before sending all currently listed IDs. Display the deleted count and reload the list.

- [ ] **Step 5: Match existing admin-page visual conventions**

  Reuse the dark/light tokens and spacing used by `resolve-metric-range-types-view.svelte`; use a warning/destructive accent for Resolve and accessible labels for all controls.

## Chunk 3: Shell integration and verification

### Task 6: Register the page in the development shell

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte:250-265`
- Modify: `web/src/lib/components/home3/content-panel.svelte:1-55, 145-260`

- [ ] **Step 1: Add the navigation item**

  Add `{ id: 'sysadmin-db-resolve-orphaned-labels', label: 'Resolve Orphaned Labels' }` under Database Maintenance.

- [ ] **Step 2: Import and render the view**

  Import `ResolveOrphanedLabelsView` and map the new child ID to it in the content panel.

- [ ] **Step 3: Make the page fill the maintenance shell**

  Add the child ID to the existing footer/overflow exceptions so the table owns its vertical scroll like the other repair pages.

### Task 7: Verify the complete change

- [ ] **Step 1: Run frontend focused tests**

  Run from `web`: `bunx vitest run src/lib/components/home3/resolve-orphaned-labels-client.test.ts`

- [ ] **Step 2: Run frontend type/build checks**

  Run from `web`: `bun run check` and the project’s existing production build command from `web/package.json`.

- [ ] **Step 3: Run backend tests and build again**

  Run from ChenWeb: `go test ./server/api/dbmainthandler -count=1` and `go build ./server/...`.

- [ ] **Step 4: Inspect the final diff and status**

  Run: `jj diff --stat`, `jj diff`, and `jj status`. Confirm only the orphan-label implementation plus the pre-existing unrelated `server/tmp` remains uncommitted.

- [ ] **Step 5: Commit the implementation**

  Use `jj commit` for the implementation paths with a focused message such as `feat: add orphaned ontology label maintenance`.
