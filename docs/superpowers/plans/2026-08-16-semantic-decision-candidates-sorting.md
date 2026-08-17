# Semantic Decision Candidates Sorting Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add server-side sorting for the eight requested Semantic Decision Candidates columns while keeping filters and pagination correct.

**Architecture:** Add a whitelist-based sort expression and direction to the existing candidate list filter. The Svelte client owns the active sort state and sends it on every paginated request; the server applies ordering before `LIMIT/OFFSET`.

**Tech Stack:** Go, PostgreSQL SQL, Echo, Svelte 5, TypeScript, svelte-check.

---

## Chunk 1: Backend sort contract

### Task 1: Add store sort fields and validation

**Files:**
- Modify: `server/api/ontology/assertions/decision_candidates_store.go`
- Test: `server/api/ontology/assertions/decision_candidates_store_test.go`

- [ ] Add `SortBy` and `SortDir` to `DecisionCandidateListFilter`.
- [ ] Add a private whitelist mapping the eight UI keys to SQL expressions and a helper that returns the validated expression/direction, defaulting to `id DESC`.
- [ ] Build the list query with `NULLS LAST` and `id` as a deterministic tie-breaker before `LIMIT/OFFSET`.
- [ ] Add focused unit tests for all supported keys, both directions, unsupported values, and null ordering/tie-breaker SQL.
- [ ] Run `go test ./server/api/ontology/assertions` from `ChenWeb` and confirm it passes.

### Task 2: Pass query parameters through the handler

**Files:**
- Modify: `server/api/kbhandler/semantic_decision_candidates_handler.go`

- [ ] Read `sort_by` and `sort_dir` from the request and pass them into the store filter.
- [ ] Preserve the current default response behavior for requests that omit sorting.
- [ ] Run the relevant handler/store tests.

## Chunk 2: Client sorting UI

### Task 3: Track sort state and request sorted pages

**Files:**
- Modify: `web/src/lib/components/home3/semantic-decision-candidates-client.ts`
- Modify: `web/src/lib/components/home3/semantic-decision-candidates-view.svelte`

- [ ] Add typed sort keys and direction to the client list request.
- [ ] Add active sort state in the view, with header activation toggling direction and resetting to page 1.
- [ ] Render the eight requested headers as accessible buttons with a visible ascending/descending indicator; leave non-requested columns unchanged.
- [ ] Ensure refresh, filters, and pagination retain the active sort.
- [ ] Run `npm run check` from `ChenWeb/web`.

## Chunk 3: Verification

### Task 4: Verify the integrated change

- [ ] Review the diff to confirm unrelated dirty-worktree files are untouched.
- [ ] Run focused Go tests and the web check.
- [ ] Run formatting/lint checks only if they do not rewrite unrelated files.
- [ ] Record the changed knowledge, affected tests/docs, and any intentionally undocumented behavior in the handoff.
