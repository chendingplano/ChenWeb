# Semantic Assertions Admin Page Implementation Plan

> **For agentic workers:** Implement this plan in the current session using the repository's existing patterns.

**Goal:** Add a full-management `kb.semantic_assertions` admin page and navigation entry under Doc Process Pipeline.

**Architecture:** Extend the assertion store and KB handlers with a focused admin list/detail/action API. Add a typed client and Svelte sibling view modeled on Semantic Decision Candidates, with revisions instead of in-place payload edits.

**Tech Stack:** Go, Echo, PostgreSQL, Svelte 5, TypeScript, Tailwind utility classes.

---

### Task 1: Add assertion admin store and API

**Files:**
- Modify: `server/api/ontology/assertions/assertions_store.go`
- Create: `server/api/kbhandler/semantic_assertions_handler.go`
- Modify: `server/api/routes.go`
- Test: focused store/handler tests alongside existing assertion and KB handler tests

- [ ] Add filtered, paginated, sortable latest/all-revision listing to `AssertionStore`.
- [ ] Add GET list/detail, POST create/revision, transition, defer, and retry handlers using the existing store methods.
- [ ] Register `/kb/semantic-assertions` routes.
- [ ] Add focused tests for list filters and handler action wiring.

### Task 2: Add the frontend client and admin view

**Files:**
- Create: `web/src/lib/components/home3/semantic-assertions-client.ts`
- Create: `web/src/lib/components/home3/semantic-assertions-view.svelte`

- [ ] Define assertion types, filters, sort keys, and API functions.
- [ ] Implement filters, pagination, sorting, refresh, create/revision form, details, lifecycle controls, defer/retry, and feedback states.
- [ ] Match the existing Semantic Decision Candidates page's visual and interaction conventions.

### Task 3: Wire navigation and verify

**Files:**
- Modify: `web/src/lib/components/home3/nav-rail.svelte`
- Modify: the page-key component switch/host where sibling admin views are selected, if required by the existing routing pattern.

- [ ] Add `Semantic Assertions` below `Doc Process Pipeline`.
- [ ] Wire the page key to the new component.
- [ ] Run Go tests for touched packages and ChenWeb frontend checks.
- [ ] Confirm unrelated pre-existing worktree changes remain untouched.
